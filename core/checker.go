package core

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"servicetelemetry/config"
)

// 新增：错误分类枚举
type ErrorType string

const (
	ErrorTypeNetwork ErrorType = "network" // 网络错误
	ErrorTypeTimeout ErrorType = "timeout" // 超时错误
	ErrorTypeSSL     ErrorType = "ssl"     // SSL证书错误
	ErrorTypeHTTP    ErrorType = "http"    // HTTP状态码错误
	ErrorTypeKeyword ErrorType = "keyword" // 关键词匹配错误
	ErrorTypeInvalid ErrorType = "invalid" // 无效地址错误
	ErrorTypeUnknown ErrorType = "unknown" // 未知错误
)

// 新增：监控结果缓存
var (
	resultCache = make(map[string]*MonitorResult)
	cacheMu     sync.RWMutex
)

// ServiceChecker 服务检查器，负责执行具体的服务可用性检查
type ServiceChecker struct {
	cfg      *config.MonitorConfig
	cacheTTL time.Duration
}

// NewServiceChecker 创建一个新的服务检查器
func NewServiceChecker(cfg *config.MonitorConfig) *ServiceChecker {
	return &ServiceChecker{
		cfg:      cfg,
		cacheTTL: cfg.CacheTTL,
	}
}

// 新增：获取缓存的监控结果
func (sc *ServiceChecker) GetCachedResult(targetURL string) (*MonitorResult, bool) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	result, ok := resultCache[targetURL]
	if !ok {
		return nil, false
	}
	// 检查缓存是否过期
	if time.Since(result.CheckedAt) > sc.cacheTTL {
		return nil, false
	}
	return result, true
}

// 新增：更新监控结果缓存
func (sc *ServiceChecker) updateCache(result *MonitorResult) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	resultCache[result.TargetURL] = result
}

// 新增：清理过期缓存
func (sc *ServiceChecker) CleanExpiredCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	// 修复点1补充：如果需要保留now变量，可改为如下写法（二选一）
	// now := time.Now()
	// for url, result := range resultCache {
	// 	if now.Sub(result.CheckedAt) > sc.cacheTTL {
	// 		delete(resultCache, url)
	// 	}
	// }
	// 推荐写法：直接使用time.Now()，删除冗余变量
	for url, result := range resultCache {
		if time.Since(result.CheckedAt) > sc.cacheTTL {
			delete(resultCache, url)
		}
	}
}

// CheckTarget 检查单个监控目标的可用性（增强版）
func (sc *ServiceChecker) CheckTarget(target *MonitorTarget) *MonitorResult {
	// 先检查缓存
	if cachedResult, ok := sc.GetCachedResult(target.URL); ok {
		return cachedResult
	}

	// 初始化监控结果
	result := &MonitorResult{
		TargetURL:  target.URL,
		CheckedAt:  time.Now(),
		StatusCode: 0,
		ErrorType:  "", // 新增字段
	}

	// 生成指数退避重试间隔
	backoff := make([]time.Duration, sc.cfg.MaxRetry)
	base := 100 * time.Millisecond
	for i := 0; i < sc.cfg.MaxRetry; i++ {
		backoff[i] = base * (1 << i)
	}

	var lastErr error
	var errType ErrorType

	// 执行重试逻辑
	for retry := 0; retry < sc.cfg.MaxRetry; retry++ {
		start := time.Now()

		// 区分TCP和HTTP/HTTPS服务
		if strings.HasPrefix(strings.ToLower(target.URL), "tcp://") {
			lastErr, errType = sc.checkTCP(target.URL, result)
		} else {
			lastErr, errType = sc.checkHTTP(target.URL, target.Keyword, result)
		}

		// 计算响应耗时
		result.ResponseTime = float64(time.Since(start).Milliseconds())

		// 检查成功
		if lastErr == nil {
			result.Status = "success"
			result.ErrorMsg = ""
			result.ErrorType = ""
			break
		}

		// 最后一次重试失败
		if retry == sc.cfg.MaxRetry-1 {
			result.Status = "failed"
			result.ErrorMsg = lastErr.Error()
			result.ErrorType = string(errType)
		} else {
			time.Sleep(backoff[retry])
		}
	}

	// 更新缓存
	sc.updateCache(result)

	return result
}

// checkTCP 检查TCP服务（增强错误分类）
func (sc *ServiceChecker) checkTCP(url string, result *MonitorResult) (error, ErrorType) {
	address := strings.TrimPrefix(url, "tcp://")
	if address == "" {
		return errors.New("无效的TCP地址，格式应为 tcp://ip:port"), ErrorTypeInvalid
	}

	// 解析地址
	_, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("解析TCP地址失败：%w", err), ErrorTypeInvalid
	}

	// 修复点2：删除未使用的port变量，或补充使用逻辑（二选一）
	// 方案1（推荐）：直接删除port变量的定义
	// 方案2：保留port变量并使用（如下）
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		return fmt.Errorf("无效的端口号：%s", portStr), ErrorTypeInvalid
	}
	// 补充使用port变量的逻辑（例如日志输出或参数传递）
	_ = port // 最简修复：使用空白标识符标记变量已使用

	// 建立TCP连接
	conn, err := net.DialTimeout("tcp", address, sc.cfg.TCPTimeout)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("TCP连接超时：%w", err), ErrorTypeTimeout
		}
		return fmt.Errorf("TCP连接失败：%w", err), ErrorTypeNetwork
	}
	defer conn.Close()

	result.StatusCode = 0
	return nil, ""
}

// checkHTTP 检查HTTP/HTTPS服务（增强错误分类）
func (sc *ServiceChecker) checkHTTP(url string, keyword string, result *MonitorResult) (error, ErrorType) {
	// 构建HTTP客户端
	client := &http.Client{
		Timeout: sc.cfg.HTTPTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12, // 强制TLS 1.2+
			},
			DisableKeepAlives: true, // 关闭长连接
		},
	}

	// 构建GET请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败：%w", err), ErrorTypeInvalid
	}

	// 添加自定义User-Agent
	req.Header.Set("User-Agent", "ServiceMonitor/1.0 (+https://github.com/example/servicemonitor)")

	// 发送HTTP请求
	resp, err := client.Do(req)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("HTTP请求超时：%w", err), ErrorTypeTimeout
		}
		if strings.Contains(err.Error(), "certificate") {
			return fmt.Errorf("SSL证书验证失败：%w", err), ErrorTypeSSL
		}
		return fmt.Errorf("HTTP请求失败：%w", err), ErrorTypeNetwork
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(io.LimitReader(resp.Body, sc.cfg.MaxBodySize))
	if err != nil {
		return fmt.Errorf("读取响应体失败：%w", err), ErrorTypeUnknown
	}

	// 记录HTTP状态码
	result.StatusCode = resp.StatusCode

	// 关键词匹配
	if keyword != "" {
		result.KeywordMatched = strings.Contains(string(body), keyword)
		if !result.KeywordMatched {
			return fmt.Errorf("响应体未找到关键词：%s", keyword), ErrorTypeKeyword
		}
	}

	// 提取SSL证书信息
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		expiry := cert.NotAfter
		days := int(expiry.Sub(time.Now()).Hours() / 24)

		if days > 0 {
			result.SSLCertExpiry = fmt.Sprintf("还有%d天过期", days)
		} else if days == 0 {
			result.SSLCertExpiry = "今日过期"
		} else {
			result.SSLCertExpiry = fmt.Sprintf("已过期%d天", -days)
		}

		// 检查证书有效期（提前预警）
		if days < 7 {
			result.Warning = fmt.Sprintf("SSL证书即将过期（剩余%d天）", days) // 新增字段
		}
	}

	// 验证HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP状态码异常：%d", resp.StatusCode), ErrorTypeHTTP
	}

	return nil, ""
}
