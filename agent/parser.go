package agent

import (
	"strings"
	"unicode"
)

// ParseQueryIntent 解析用户查询内容，提取查询意图和条件
// userQuery：用户输入的查询内容
// defaultTimeRange：默认检索时间范围（小时）
func ParseQueryIntent(userQuery string, defaultTimeRange int) *QueryIntent {
	// 初始化查询意图，设置默认时间范围
	intent := &QueryIntent{
		TimeRangeHours: defaultTimeRange,
		TargetKeywords: []string{},
	}

	if userQuery == "" {
		return intent
	}

	// 转换为小写，统一查询条件判断标准
	lowerQuery := strings.ToLower(userQuery)

	// 解析查询意图：是否查询失败服务
	failedKeywords := []string{"挂了", "异常", "失败", "failed", "error", "超时"}
	for _, kw := range failedKeywords {
		if strings.Contains(lowerQuery, kw) {
			intent.IsFailed = true
			break
		}
	}

	// 解析查询意图：是否查询SSL证书相关信息
	sslKeywords := []string{"ssl", "证书", "过期", "有效期"}
	for _, kw := range sslKeywords {
		if strings.Contains(lowerQuery, kw) {
			intent.IsSSL = true
			break
		}
	}

	// 解析查询意图：是否查询TCP服务相关信息
	if strings.Contains(lowerQuery, "tcp") {
		intent.IsTCP = true
	}

	// 提取目标地址关键词，用于过滤结果
	targetPatterns := []string{"baidu", "github", "google", "127.0.0.1", "localhost"}
	for _, pat := range targetPatterns {
		if strings.Contains(lowerQuery, pat) {
			intent.TargetKeywords = append(intent.TargetKeywords, pat)
		}
	}

	// 提取用户指定的时间范围，覆盖默认值
	intent.TimeRangeHours = extractTimeRange(lowerQuery, defaultTimeRange)

	return intent
}

// extractTimeRange 提取查询内容中的时间范围，支持「近N小时」「近N天」格式
// query：小写格式的用户查询内容
// defaultRange：默认时间范围（小时）
func extractTimeRange(query string, defaultRange int) int {
	// 提取查询中的数字部分
	var numStr string
	for _, c := range query {
		if unicode.IsDigit(c) {
			numStr += string(c)
		} else if numStr != "" {
			break
		}
	}

	// 无数字，返回默认时间范围
	if numStr == "" {
		return defaultRange
	}

	// 转换数字字符串为整数
	num := 0
	for _, c := range numStr {
		num = num*10 + int(c-'0')
	}

	// 处理「天」单位，转换为小时
	if strings.Contains(query, "天") {
		return num * 24
	}

	// 处理「小时」单位，直接返回
	if strings.Contains(query, "小时") {
		return num
	}

	// 无明确单位，返回默认时间范围
	return defaultRange
}
