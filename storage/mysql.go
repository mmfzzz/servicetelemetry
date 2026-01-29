package storage

import (
	"database/sql"
	"fmt"
	"time"

	"servicetelemetry/config"
	"servicetelemetry/core"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLStorage MySQL存储客户端，负责监控数据的持久化和查询
type MySQLStorage struct {
	db *sql.DB // 数据库连接对象，用于执行SQL操作
}

// NewMySQLStorage 创建一个新的MySQL存储客户端，自动创建数据库和数据表
// cfg：数据库配置结构体指针，提供连接所需的参数
func NewMySQLStorage(cfg *config.DBConfig) (*MySQLStorage, error) {
	// 构建不指定数据库的DSN，用于连接MySQL服务端（创建数据库）
	dsnWithoutDB := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port)

	// 连接MySQL服务端（不指定具体数据库）
	dbServer, err := sql.Open("mysql", dsnWithoutDB)
	if err != nil {
		return nil, fmt.Errorf("连接MySQL服务端失败：%w", err)
	}
	defer dbServer.Close() // 数据库创建完成后关闭该连接

	// 自动创建数据库（若不存在），使用utf8mb4编码兼容所有字符
	createDBSQL := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", cfg.DBName)
	_, err = dbServer.Exec(createDBSQL)
	if err != nil {
		return nil, fmt.Errorf("创建数据库 %s 失败：%w", cfg.DBName, err)
	}

	// 构建指定数据库的DSN，用于连接目标数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)

	// 打开目标数据库连接
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接数据库 %s 失败：%w", cfg.DBName, err)
	}

	// 配置数据库连接池，优化连接复用和资源占用
	db.SetMaxOpenConns(20)                  // 最大打开连接数
	db.SetMaxIdleConns(10)                  // 最大空闲连接数
	db.SetConnMaxLifetime(60 * time.Minute) // 连接最大存活时间
	db.SetConnMaxIdleTime(30 * time.Second)
	// 测试数据库连接是否可用
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("数据库 %s Ping失败：%w", cfg.DBName, err)
	}

	// 自动初始化数据表（若不存在）
	if err := initTables(db); err != nil {
		return nil, fmt.Errorf("初始化表失败：%w", err)
	}

	return &MySQLStorage{db: db}, nil
}

// initTables 初始化数据表，创建监控结果表和监控目标表
// db：数据库连接对象
func initTables(db *sql.DB) error {
	// 创建监控结果表
	resultTableSQL := `
	CREATE TABLE IF NOT EXISTS monitor_results (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		target_url VARCHAR(255) NOT NULL,
		status VARCHAR(20) NOT NULL,
		status_code INT DEFAULT 0,
		response_time FLOAT DEFAULT 0,
		ssl_cert_expiry VARCHAR(50) DEFAULT '',
		keyword_matched TINYINT(1) DEFAULT 0,
		error_msg VARCHAR(512) DEFAULT '',
		checked_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	// 创建监控目标表
	targetTableSQL := `
	CREATE TABLE IF NOT EXISTS monitor_targets (
		id INT AUTO_INCREMENT PRIMARY KEY,
		target_url VARCHAR(255) NOT NULL UNIQUE,
		keyword VARCHAR(100) DEFAULT '',
		is_current TINYINT(1) DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	// 执行建表语句
	if _, err := db.Exec(resultTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec(targetTableSQL); err != nil {
		return err
	}

	return nil
}

// SaveResult 保存监控结果到数据库
// result：监控结果结构体指针
func (ms *MySQLStorage) SaveResult(result *core.MonitorResult) error {
	sql := `
    INSERT INTO monitor_results (
        target_url, status, status_code, response_time,
        ssl_cert_expiry, keyword_matched, error_msg, checked_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `

	// 执行SQL时，打印参数（便于调试）
	_, err := ms.db.Exec(
		sql,
		result.TargetURL,
		result.Status,
		result.StatusCode,
		result.ResponseTime,
		result.SSLCertExpiry,
		result.KeywordMatched,
		result.ErrorMsg,
		result.CheckedAt,
	)
	if err != nil {
		return fmt.Errorf("执行SaveResult SQL失败：%w", err)
	}
	return nil
}

// SaveTarget 保存监控目标到数据库（存在则更新，不存在则插入）
// target：监控目标结构体指针
func (ms *MySQLStorage) SaveTarget(target *core.MonitorTarget) error {
	sql := `
	INSERT INTO monitor_targets (target_url, keyword, is_current)
	VALUES (?, ?, ?)
	ON DUPLICATE KEY UPDATE keyword=?, is_current=?
	`

	_, err := ms.db.Exec(
		sql,
		target.URL,
		target.Keyword,
		target.IsCurrent,
		target.Keyword,
		target.IsCurrent,
	)

	return err
}

// QueryResults 按条件查询监控结果，支持时间范围和目标地址过滤
// targetURL：目标地址模糊查询关键词（可选）
// startTime：查询开始时间
// endTime：查询结束时间
// limit：返回结果最大条数
// QueryResults 按条件查询监控结果
func (ms *MySQLStorage) QueryResults(targetURL string, startTime, endTime time.Time, limit int) ([]*core.MonitorResult, error) {
	sql := `
    SELECT id, target_url, status, status_code, response_time,
           ssl_cert_expiry, keyword_matched, error_msg, checked_at
    FROM monitor_results
    WHERE checked_at BETWEEN ? AND ?
    `
	args := []interface{}{startTime, endTime}

	// 打印查询条件（便于调试）
	fmt.Printf("查询条件：targetURL=%s, startTime=%s, endTime=%s\n", targetURL, startTime, endTime)

	if targetURL != "" {
		sql += " AND target_url LIKE ?"
		args = append(args, "%"+targetURL+"%")
	}

	sql += " ORDER BY status DESC, checked_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := ms.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("执行QueryResults SQL失败：%w", err)
	}
	defer rows.Close()

	var results []*core.MonitorResult
	for rows.Next() {
		var r core.MonitorResult
		err := rows.Scan(
			&r.ID,
			&r.TargetURL,
			&r.Status,
			&r.StatusCode,
			&r.ResponseTime,
			&r.SSLCertExpiry,
			&r.KeywordMatched,
			&r.ErrorMsg,
			&r.CheckedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描结果失败：%w", err)
		}
		results = append(results, &r)
	}

	return results, nil
}

// Close 关闭数据库连接，释放资源
func (ms *MySQLStorage) Close() error {
	return ms.db.Close()
}
