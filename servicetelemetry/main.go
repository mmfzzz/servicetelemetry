package main

import (
	"servicetelemetry/agent"
	"servicetelemetry/api"
	"servicetelemetry/config"
	"servicetelemetry/core"
	"servicetelemetry/storage"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 1. 加载配置（支持热加载）
	cfg := config.DefaultConfig()
	config.StartConfigHotReload(30 * time.Second) // 每30秒检查一次配置更新

	// 2. 初始化数据库存储客户端
	mysqlStorage, err := storage.NewMySQLStorage(&cfg.DB)
	if err != nil {
		panic("初始化数据库失败：" + err.Error())
	}
	defer mysqlStorage.Close()

	// 3. 初始化核心服务检查器
	checker := core.NewServiceChecker(&cfg.Monitor)

	// 4. 定期清理过期缓存
	go func() {
		ticker := time.NewTicker(cfg.Monitor.CacheTTL)
		for range ticker.C {
			checker.CleanExpiredCache()
		}
	}()

	// 5. 初始化小助手数据检索器
	retriever := agent.NewDataRetriever(mysqlStorage, &cfg.Agent)

	// 6. 初始化HTTP接口处理器
	handler := api.NewHandler(checker, mysqlStorage, retriever, cfg)

	// 7. 初始化Gin引擎
	router := gin.Default()

	// 配置静态文件路由
	router.Static("/static", "./static")

	// 8. 注册API路由
	handler.RegisterRoutes(router)

	// 9. 启动HTTP服务
	println("服务启动成功，访问 http://localhost:8080/static 查看监控大屏")
	println("配置热加载已启用（30秒间隔）")
	if err := router.Run(":8080"); err != nil {
		panic("服务启动失败：" + err.Error())
	}
}
