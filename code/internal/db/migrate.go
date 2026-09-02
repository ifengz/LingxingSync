// Package db 数据访问层 —— migrate.go 负责数据库 schema 迁移。
//
// 设计：
//   - 迁移 SQL 文件放在 migrations/ 目录（main 传入路径，如 "migrations"）
//   - 所有 CREATE TABLE 用 IF NOT EXISTS，天然幂等，可重复执行
//   - DSN 已开 multiStatements=true，可直接把整个 .sql 文件喂给 db.Exec
//   - 按文件名升序执行（001_system.sql → 002_data_tables.sql → ...）
package db

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

// RunMigrations 读取 dir 下所有 *.sql（按完整文件名升序），只执行尚未记录的文件。
// 迁移 SQL 仍须保持可重试；MySQL 多语句 DDL 不能依赖事务回滚。
func RunMigrations(db *sqlx.DB, dir string) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename VARCHAR(255) NOT NULL,
		checksum CHAR(64) NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (filename)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("db.RunMigrations: 创建 schema_migrations 失败: %w", err)
	}

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

	applied := make(map[string]string, len(files))
	rows, err := db.Queryx("SELECT filename, checksum FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("db.RunMigrations: 读取 schema_migrations 失败: %w", err)
	}
	for rows.Next() {
		var filename, checksum string
		if err := rows.Scan(&filename, &checksum); err != nil {
			return fmt.Errorf("db.RunMigrations: 读取 schema_migrations 记录失败: %w", err)
		}
		applied[filename] = checksum
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("db.RunMigrations: 遍历 schema_migrations 失败: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("db.RunMigrations: 关闭 schema_migrations 查询失败: %w", err)
	}

	for _, name := range files {
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("db.RunMigrations: 读 %s 失败: %w", path, err)
		}
		checksum := migrationChecksum(raw)
		if recorded, ok := applied[name]; ok {
			if recorded != checksum {
				return fmt.Errorf("db.RunMigrations: 已执行迁移 %s 的 checksum 不匹配", name)
			}
			continue
		}
		stmt := string(raw)
		if strings.TrimSpace(stmt) != "" {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("db.RunMigrations: 执行 %s 失败: %w", name, err)
			}
		}
		if _, err := db.Exec("INSERT INTO schema_migrations (filename, checksum) VALUES (?, ?)", name, checksum); err != nil {
			return fmt.Errorf("db.RunMigrations: 记录 %s 失败: %w", name, err)
		}
	}
	return nil
}

func migrationChecksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
