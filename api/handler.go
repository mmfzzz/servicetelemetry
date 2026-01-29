package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"servicetelemetry/agent"
	"servicetelemetry/config"
	"servicetelemetry/core"
	"servicetelemetry/storage"

	"github.com/gin-gonic/gin"
)

// 改造Handler结构体，新增summarizer字段
type Handler struct {
	checker    *core.ServiceChecker
	storage    *storage.MySQLStorage
	retriever  *agent.DataRetriever
	cfg        *config.GlobalConfig
	summarizer *agent.LightweightSummarizer // 新增：小助手AI实例
}

// 改造NewHandler，初始化summarizer
func NewHandler(
	checker *core.ServiceChecker,
	storage *storage.MySQLStorage,
	retriever *agent.DataRetriever,
	cfg *config.GlobalConfig,
) *Handler {
	return &Handler{
		checker:    checker,
		storage:    storage,
		retriever:  retriever,
		cfg:        cfg,
		summarizer: agent.NewLightweightSummarizer(&cfg.Agent), // 初始化AI实例
	}
}

// 保留原有SubmitTargets方法（仅修复并发写问题，其余不变）
func (h *Handler) SubmitTargets(c *gin.Context) {
	type TargetRequest struct {
		Targets []string `json:"targets" binding:"required"`
		Keyword string   `json:"keyword"`
	}

	var req TargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误：" + err.Error()})
		return
	}

	limiter := core.NewConcurrencyLimiter(h.cfg.Monitor.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []*core.MonitorResult

	wg.Add(len(req.Targets))
	for _, url := range req.Targets {
		limiter.Acquire()
		go func(u string) {
			defer limiter.Release()
			defer wg.Done()

			target := &core.MonitorTarget{
				URL:       u,
				Keyword:   req.Keyword,
				IsCurrent: true,
			}

			result := h.checker.CheckTarget(target)
			if err := h.storage.SaveTarget(target); err != nil {
				fmt.Printf("保存目标[%s]失败：%v\n", u, err)
			}
			if err := h.storage.SaveResult(result); err != nil {
				fmt.Printf("保存结果[%s]失败：%v\n", u, err)
			} else {
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}
		}(url)
	}

	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"message": "检查完成",
		"results": results,
	})
}

// 改造AgentQuery方法，支持通用问答
func (h *Handler) AgentQuery(c *gin.Context) {
	type AgentQueryRequest struct {
		UserQuery string `json:"userQuery" binding:"required"`
		Mode      string `json:"mode" binding:"required"`
	}

	var req AgentQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"isSuccess": false,
			"errorMsg":  "参数错误：" + err.Error(),
		})
		return
	}

	// 模式1：data - 纯监控数据查询（原有功能，无修改）
	if req.Mode == "data" {
		intent := agent.ParseQueryIntent(req.UserQuery, h.cfg.Agent.DefaultTimeRange)
		data, err := h.retriever.Retrieve(intent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"isSuccess": false,
				"errorMsg":  "数据检索失败：" + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"isSuccess": true,
			"data":      data,
			"queryTime": time.Now(),
		})
		return
	}

	// 模式2：ai - 分【监控总结】/【通用问答】，显式区分（核心改造）
	if req.Mode == "ai" {
		// 第一步：检查是否有/chat前缀（优先）
		userQueryTrim := strings.TrimSpace(req.UserQuery)
		isGeneralChat := strings.HasPrefix(userQueryTrim, "/chat")
		realQuery := userQueryTrim

		if isGeneralChat {
			// 去除/chat前缀
			realQuery = strings.TrimPrefix(userQueryTrim, "/chat")
			realQuery = strings.TrimSpace(realQuery)
			if realQuery == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"isSuccess": false,
					"errorMsg":  "通用问答请输入/chat 加具体问题，例如：/chat 什么是HTTP 502？",
				})
				return
			}
		} else {
			// 第二步：无/chat前缀，但通过关键词识别通用问答（双重保险）
			generalKeywords := []string{"如何", "什么是", "区别", "为什么", "怎么", "教程", "含义", "原理", "步骤"}
			for _, kw := range generalKeywords {
				if strings.Contains(userQueryTrim, kw) {
					isGeneralChat = true
					realQuery = userQueryTrim
					break
				}
			}
		}

		// 通用问答逻辑（带前缀或匹配关键词）
		if isGeneralChat {
			chatReply, err := h.summarizer.Chat(realQuery)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"isSuccess": false,
					"errorMsg":  "小助手回答失败：" + err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"isSuccess":        true,
				"reply":            chatReply,
				"isMonitorSummary": false,
				"queryTime":        time.Now(),
			})
			return
		}

		// 无前缀且不匹配通用关键词 → 监控总结逻辑
		intent := agent.ParseQueryIntent(req.UserQuery, h.cfg.Agent.DefaultTimeRange)
		monitorData, err := h.retriever.Retrieve(intent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"isSuccess": false,
				"errorMsg":  "监控数据检索失败：" + err.Error(),
			})
			return
		}
		if len(monitorData) > 0 {
			summary, err := h.summarizer.Summarize(monitorData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"isSuccess": false,
					"errorMsg":  "监控数据总结失败：" + err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"isSuccess":        true,
				"reply":            summary,
				"isMonitorSummary": true,
				"queryTime":        time.Now(),
			})
			return
		}
		// 无监控数据提示
		c.JSON(http.StatusOK, gin.H{
			"isSuccess":        true,
			"reply":            "未查询到相关监控数据，若需通用问答，请在问题前加/chat 前缀（例：/chat 什么是Goroutine？）",
			"isMonitorSummary": false,
			"queryTime":        time.Now(),
		})
		return
	}

	// 未知模式提示
	c.JSON(http.StatusBadRequest, gin.H{
		"isSuccess": false,
		"errorMsg":  "不支持的查询模式，仅支持 data 和 ai",
	})
}

// 保留原有GetHistoryResults方法（不变）
func (h *Handler) GetHistoryResults(c *gin.Context) {
	targetURL := c.Query("targetUrl")
	startTimeStr := c.Query("startTime")
	endTimeStr := c.Query("endTime")

	var startTime, endTime time.Time
	endTime = time.Now()
	var err error

	if startTimeStr != "" {
		startTime, err = time.Parse("2006-01-02 15:04:05", startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "开始时间格式错误，应为：2006-01-02 15:04:05"})
			return
		}
	} else {
		startTime = endTime.Add(-24 * time.Hour)
	}

	if endTimeStr != "" {
		endTime, err = time.Parse("2006-01-02 15:04:05", endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "结束时间格式错误，应为：2006-01-02 15:04:05"})
			return
		}
	}

	results, err := h.storage.QueryResults(targetURL, startTime, endTime, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询历史数据失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(results),
		"list":  results,
	})
}

// 保留原有RegisterRoutes方法（不变）
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	apiGroup := router.Group("/api")
	{
		apiGroup.POST("/targets", h.SubmitTargets)
		apiGroup.POST("/agent/query", h.AgentQuery)
		apiGroup.GET("/history/results", h.GetHistoryResults)
	}
}
