// Package db 数据访问层 —— migrate.go 负责数据库 schema 迁移。
//
// 设计：
//   - 迁移 SQL 文件放在 migrations/ 目录（main 传入路径，如 "migrations"）
//   - 所有 CREATE TABLE 用 IF NOT EXISTS，天然幂等，可重复执行
//   - DSN 已开 multiStatements=true，可直接把整个 .sql 文件喂给 db.Exec
//   - 按文件名升序执行（001_system.sql → 002_data_tables.sql → ...）
package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

// RunMigrations 读取 dir 下所有 *.sql（按文件名升序），逐文件整段 Exec。
//
// 幂等性依赖 SQL 里的 IF NOT EXISTS / IF EXISTS，不维护 schema_versions 表——
// 宪法 §2 明确这是单进程同步机，不需要演进式迁移，只需要「启动时把表建好」。
//
// 任何文件 Exec 失败立刻返回 error（fail-loud，启动断言）。
func RunMigrations(db *sqlx.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("db.RunMigrations: 读迁移目录 %s 失败: %w", dir, err)
	}

	// 收集 .sql 文件，按文件名升序。
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	if len(files) == 0 {
		return fmt.Errorf("db.RunMigrations: 目录 %s 下没有任何 .sql 文件", dir)
	}

	for _, name := range files {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("db.RunMigrations: 读 %s 失败: %w", path, err)
		}
		stmt := string(raw)
		if strings.TrimSpace(stmt) == "" {
			continue // 空文件跳过，不报错
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("db.RunMigrations: 执行 %s 失败: %w", name, err)
		}
	}
	return nil
}
