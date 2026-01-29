package agent

import (
	"time"

	"servicetelemetry/core"
)

// AgentQuery 小助手查询请求结构体，接收前端传入的查询参数
type AgentQuery struct {
	UserQuery string `json:"userQuery"` // 用户查询内容
	Mode      string `json:"mode"`      // 响应模式，data（纯数据）/ai（总结数据）
}

// AgentResponse 小助手响应结果结构体，返回给前端的查询结果
type AgentResponse struct {
	Summary   string                `json:"summary"`   // 数据总结（可选，仅AI模式返回）
	Data      []*core.MonitorResult `json:"data"`      // 结构化监控数据，核心返回结果
	QueryTime time.Time             `json:"queryTime"` // 查询完成时间
	Mode      string                `json:"mode"`      // 响应模式，与请求模式一致
	IsSuccess bool                  `json:"isSuccess"` // 查询是否成功
	ErrorMsg  string                `json:"errorMsg"`  // 错误信息，查询失败时返回
}

// QueryIntent 查询意图结构体，存储解析后的用户查询条件
type QueryIntent struct {
	IsFailed       bool     // 是否查询失败服务
	IsSSL          bool     // 是否查询SSL证书相关信息
	IsTCP          bool     // 是否查询TCP服务相关信息
	TargetKeywords []string // 目标地址关键词，用于过滤结果
	TimeRangeHours int      // 检索时间范围（小时）
}
