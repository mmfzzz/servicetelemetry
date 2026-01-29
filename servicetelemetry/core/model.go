package core

import (
	"time"
)

// MonitorTarget 监控目标结构体
type MonitorTarget struct {
	URL       string `json:"url"`       // 目标服务地址
	Keyword   string `json:"keyword"`   // 响应体匹配关键词
	IsCurrent bool   `json:"isCurrent"` // 是否为当前有效监控目标
	Priority  string `json:"priority"`  // 新增：任务优先级（low/normal/high）
}

// MonitorResult 监控结果结构体（增强版）
type MonitorResult struct {
	ID             uint64    `json:"id"`             // 结果唯一标识
	TargetURL      string    `json:"targetUrl"`      // 对应监控目标的地址
	Status         string    `json:"status"`         // 检查状态
	StatusCode     int       `json:"statusCode"`     // HTTP状态码
	ResponseTime   float64   `json:"responseTime"`   // 响应耗时（毫秒）
	SSLCertExpiry  string    `json:"sslCertExpiry"`  // SSL证书过期信息
	KeywordMatched bool      `json:"keywordMatched"` // 关键词匹配结果
	ErrorMsg       string    `json:"errorMsg"`       // 错误信息
	ErrorType      string    `json:"errorType"`      // 新增：错误类型
	Warning        string    `json:"warning"`        // 新增：警告信息
	CheckedAt      time.Time `json:"checkedAt"`      // 检查完成时间
	CreatedAt      time.Time `json:"createdAt"`      // 结果入库时间
}
