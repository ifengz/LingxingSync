// Package db 是数据访问层：唯一与 MySQL 打交道的入口。
//
// 本文件 pool.go 负责：
//   - 构造 sqlx.DB 连接池（go-sql-driver/mysql DSN）
//   - 设置连接池参数（MaxOpen / MaxIdle / ConnMaxLifetime）
//   - DSN 强制 UTC（宪法 §4：所有 DATETIME 存 UTC），并对 user/password 做 URL 转义
//     以支持含特殊字符（/, =, @, : 等）的凭证。
package db

import (
	"fmt"
	"net/url"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/config"

	// 注册 mysql driver。
	_ "github.com/go-sql-driver/mysql"
)

// NewPool 根据 config.Database 建立并返回一个 sqlx.DB 连接池。
//
// DSN 固定参数：
//   - parseTime=true：把 MySQL DATETIME 扫成 time.Time
//   - loc=UTC：宪法 §4，所有时间按 UTC 解释
//   - charset=utf8mb4：支持 emoji / 4 字节字符
//   - timeout=10s：建连超时
//   - multiStatements=true：migrate.go 整文件 Exec 依赖它
//
// user / password 用 url.QueryEscape 转义，避免密码里出现 / @ : = 等把 DSN 拆坏。
func NewPool(cfg config.Database) (*sqlx.DB, error) {
	if cfg.Port == 0 {
		return nil, fmt.Errorf("db.NewPool: cfg.Port 不能为 0")
	}
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC&charset=utf8mb4&timeout=10s&multiStatements=true",
		url.QueryEscape(cfg.User),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		cfg.DB,
	)

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.NewPool: 连接 MySQL %s@%s:%d/%s 失败: %w",
			cfg.User, cfg.Host, cfg.Port, cfg.DB, err)
	}

	// 连接池参数（宪法 §4：单进程，每「账号+接口」一 Worker，不要过度开连接）。
	maxOpen := cfg.MaxOpen
	if maxOpen <= 0 {
		maxOpen = 20
	}
	maxIdle := cfg.MaxIdle
	if maxIdle <= 0 {
		maxIdle = 5
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	// ConnMaxLifetime 取连接超时的若干倍，避免长连接被 MySQL wait_timeout 干掉后还在用。
	db.SetConnMaxLifetime(time.Hour)

	return db, nil
}
