package config

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// GlobalConfig 全局配置结构体，包含所有模块的配置信息
type GlobalConfig struct {
	Monitor MonitorConfig `json:"monitor"` // 服务监控配置
	DB      DBConfig      `json:"db"`      // 数据库配置
	Agent   AgentConfig   `json:"agent"`   // 小助手配置
}

// MonitorConfig 服务监控配置，控制检查的并发、超时等参数
type MonitorConfig struct {
	Concurrency   int           `json:"concurrency"`   // 最大并发检查数，避免同时请求过多目标
	CheckInterval time.Duration `json:"checkInterval"` // 监控检查间隔，定时刷新监控结果
	HTTPTimeout   time.Duration `json:"httpTimeout"`   // HTTP请求超时时间
	TCPTimeout    time.Duration `json:"tcpTimeout"`    // TCP连接超时时间
	MaxRetry      int           `json:"maxRetry"`      // 目标检查失败后的最大重试次数
	MaxBodySize   int64         `json:"maxBodySize"`   // HTTP响应体最大读取大小，防止内存溢出（1MB）
	LogLevel      string        `json:"logLevel"`      // 新增：日志级别
	CacheTTL      time.Duration `json:"cacheTTL"`      // 新增：监控结果缓存过期时间
}

// DBConfig 数据库配置，用于连接MySQL数据库
type DBConfig struct {
	Host     string `json:"host"`     // 数据库地址
	Port     int    `json:"port"`     // 数据库端口
	User     string `json:"user"`     // 数据库用户名
	Password string `json:"password"` // 数据库密码
	DBName   string `json:"dbName"`   // 数据库名称
	MaxOpen  int    `json:"maxOpen"`  // 新增：最大打开连接数
	MaxIdle  int    `json:"maxIdle"`  // 新增：最大空闲连接数
}

// AgentConfig 小助手配置，控制数据检索和AI总结的相关参数
type AgentConfig struct {
	EnableAI         bool      `json:"enableAI"`         // 是否开启AI总结功能
	MaxRetrieve      int       `json:"maxRetrieve"`      // 最大检索数据条数，避免返回过多数据
	DefaultTimeRange int       `json:"defaultTimeRange"` // 默认检索时间范围（小时），默认查询近24小时数据
	LLM              LLMConfig `json:"llm"`              // LLM 配置，用于AI总结功能
}

// LLMConfig LLM 模型配置，适配 DeepSeek/OpenAI 等兼容 OpenAI API 格式的模型
type LLMConfig struct {
	APIKey      string        `json:"apiKey"`      // LLM 平台 API 密钥
	APIBaseURL  string        `json:"apiBaseURL"`  // LLM 平台 API 基础地址
	ModelName   string        `json:"modelName"`   // LLM 模型名称
	Timeout     time.Duration `json:"timeout"`     // LLM 请求超时时间
	Temperature float32       `json:"temperature"` // LLM 生成温度
}

// 新增：配置热加载相关
var (
	globalConfig *GlobalConfig
	configMu     sync.RWMutex
	configFile   = "config.json"
)

// DefaultConfig 初始化全局默认配置
func DefaultConfig() *GlobalConfig {
	return &GlobalConfig{
		Monitor: MonitorConfig{
			Concurrency:   5,
			CheckInterval: 5 * time.Second,
			HTTPTimeout:   10 * time.Second,
			TCPTimeout:    5 * time.Second,
			MaxRetry:      3,
			MaxBodySize:   1024 * 1024,
			LogLevel:      "info",           // 新增
			CacheTTL:      30 * time.Second, // 新增
		},
		DB: DBConfig{
			Host:     "127.0.0.1",
			Port:     3306,
			User:     "root",
			Password: "123456",
			DBName:   "servicemonitor",
			MaxOpen:  10, // 新增
			MaxIdle:  5,  // 新增
		},
		Agent: AgentConfig{
			EnableAI:         true,
			MaxRetrieve:      50,
			DefaultTimeRange: 24,
			LLM: LLMConfig{
				APIKey:      "sk-53438aee1ecf4910aefd9815f19dd2d3",
				APIBaseURL:  "https://api.deepseek.com/v1",
				ModelName:   "deepseek-chat",
				Timeout:     30 * time.Second,
				Temperature: 0.7,
			},
		},
	}
}

// 新增：从文件加载配置
func LoadConfigFromFile(filePath string) (*GlobalConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	configMu.Lock()
	globalConfig = &cfg
	configFile = filePath
	configMu.Unlock()

	return &cfg, nil
}

// 新增：获取当前配置（线程安全）
func GetCurrentConfig() *GlobalConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	if globalConfig == nil {
		return DefaultConfig()
	}
	return globalConfig
}

// 新增：配置热加载
func StartConfigHotReload(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if _, err := LoadConfigFromFile(configFile); err != nil {
				// 仅打印错误，不中断服务
				// 实际场景可接入日志系统
				continue
			}
		}
	}()
}
