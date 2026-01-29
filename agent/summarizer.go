package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"servicetelemetry/config"
	"servicetelemetry/core"

	"github.com/sashabaranov/go-openai"
)

// 保留原有结构体，兼容历史功能
type LightweightSummarizer struct {
	client *openai.Client
	cfg    *config.LLMConfig
	enable bool
}

// 保留原有初始化方法
func NewLightweightSummarizer(agentCfg *config.AgentConfig) *LightweightSummarizer {
	if agentCfg == nil || !agentCfg.EnableAI {
		return &LightweightSummarizer{enable: false}
	}

	openaiCfg := openai.DefaultConfig(agentCfg.LLM.APIKey)
	openaiCfg.BaseURL = agentCfg.LLM.APIBaseURL

	return &LightweightSummarizer{
		client: openai.NewClientWithConfig(openaiCfg),
		cfg:    &agentCfg.LLM,
		enable: true,
	}
}

// 保留原有监控数据总结方法（兼容历史功能）
func (ls *LightweightSummarizer) Summarize(results []*core.MonitorResult) (string, error) {
	if !ls.enable || len(results) == 0 {
		return "暂无监控数据可总结。", nil
	}

	// 统计监控数据
	failedCount := 0
	var failedTargets []string
	sslExpired := []string{}

	for _, r := range results {
		if r.Status == "failed" {
			failedCount++
			failedTargets = append(failedTargets, r.TargetURL)
		}
		if r.SSLCertExpiry == "已过期" || r.SSLCertExpiry == "即将过期" {
			sslExpired = append(sslExpired, r.TargetURL)
		}
	}

	// 构建总结Prompt
	prompt := fmt.Sprintf(`
请简洁总结以下监控数据，要求：
1.  正常服务和异常服务分开说明
2.  突出SSL证书问题
3.  3句话以内，语言精炼
监控数据：
- 总监控服务数：%d
- 异常服务数：%d，异常地址：%s
- SSL证书异常地址：%s
`, len(results), failedCount, strings.Join(failedTargets, "、"), strings.Join(sslExpired, "、"))

	// 调用LLM
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:       ls.cfg.ModelName,
		Temperature: 0.3,
		MaxTokens:   200,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "你是运维监控总结助手，仅基于提供的数据进行总结，不编造额外信息。"},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	}

	resp, err := ls.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("总结失败：%w", err)
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// 新增：通用问答方法（不依赖任何监控数据，支持任意问题）
func (ls *LightweightSummarizer) Chat(userQuery string) (string, error) {
	// 未开启AI功能的提示
	if !ls.enable {
		return "小助手AI功能未开启，请在配置文件中启用EnableAI并配置正确的LLM参数后重试。", nil
	}

	// 空查询过滤
	if strings.TrimSpace(userQuery) == "" {
		return "请输入具体的问题哦～", nil
	}

	// 构建通用问答的Prompt，放开LLM的推理限制
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:       ls.cfg.ModelName,
		Temperature: 0.7, // 适度提高温度，让回答更灵活
		MaxTokens:   500, // 增大令牌数，支持更长回答
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: `你是一个全能智能小助手，能够回答用户提出的任意问题，包括但不限于：
1.  运维技术问题（HTTP状态码、TCP排查、SSL证书等）
2.  编程语言知识（Golang、Python等）
3.  通用生活常识、科普知识
4.  工作效率技巧、工具使用
回答要求：语言简洁易懂，逻辑清晰，避免冗余，针对技术问题可适当补充实操步骤。`,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userQuery,
			},
		},
	}

	// 调用LLM获取通用回答
	resp, err := ls.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("小助手回答失败：%w", err)
	}

	if len(resp.Choices) == 0 {
		return "未获取到有效回答，请稍后再试。", nil
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
