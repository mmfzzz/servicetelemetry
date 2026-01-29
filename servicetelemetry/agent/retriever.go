package agent

import (
	"time"

	"servicetelemetry/config"
	"servicetelemetry/core"
	"servicetelemetry/storage"
)

// DataRetriever 数据检索器，负责根据查询意图从数据库中提取相关监控数据
type DataRetriever struct {
	storage *storage.MySQLStorage // 数据库存储客户端，用于执行查询操作
	cfg     *config.AgentConfig   // 小助手配置，提供检索参数限制
}

// NewDataRetriever 创建一个新的数据检索器
// storage：数据库存储客户端指针
// cfg：小助手配置结构体指针
func NewDataRetriever(storage *storage.MySQLStorage, cfg *config.AgentConfig) *DataRetriever {
	return &DataRetriever{
		storage: storage,
		cfg:     cfg,
	}
}

// Retrieve 根据查询意图检索相关监控数据，返回过滤后的结果
// intent：解析后的查询意图结构体指针
func (dr *DataRetriever) Retrieve(intent *QueryIntent) ([]*core.MonitorResult, error) {
	// 构建检索时间范围：当前时间向前推指定小时数
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(intent.TimeRangeHours) * time.Hour)

	// 提取目标关键词（取第一个关键词，简化过滤逻辑）
	targetKeyword := ""
	if len(intent.TargetKeywords) > 0 {
		targetKeyword = intent.TargetKeywords[0]
	}

	// 从数据库中查询符合时间范围和目标关键词的数据
	results, err := dr.storage.QueryResults(
		targetKeyword,
		startTime,
		endTime,
		dr.cfg.MaxRetrieve,
	)
	if err != nil {
		return nil, err
	}

	// 对查询结果进行二次过滤，匹配用户具体查询意图
	filtered := dr.filterResults(results, intent)

	return filtered, nil
}

// filterResults 对数据库查询结果进行二次过滤，精准匹配查询意图
// results：数据库查询返回的原始结果
// intent：解析后的查询意图结构体指针
func (dr *DataRetriever) filterResults(results []*core.MonitorResult, intent *QueryIntent) []*core.MonitorResult {
	if len(results) == 0 {
		return results
	}

	var filtered []*core.MonitorResult
	for _, r := range results {
		// 过滤失败服务：仅保留状态为failed的结果
		if intent.IsFailed && r.Status != "failed" {
			continue
		}

		// 过滤SSL证书相关：仅保留有SSL证书信息的结果
		if intent.IsSSL && r.SSLCertExpiry == "" {
			continue
		}

		// 过滤TCP服务相关：仅保留状态码为0的结果（TCP服务无HTTP状态码）
		if intent.IsTCP && r.StatusCode != 0 {
			continue
		}

		// 符合所有过滤条件，加入结果集
		filtered = append(filtered, r)
	}

	return filtered
}
