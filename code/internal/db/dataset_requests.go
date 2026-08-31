// Package db 数据访问层 —— dataset_requests.go 数据集请求日志（dataset_request_logs）。
//
// 单写者原则（宪法 §8）：datasetapi.Handler 的 RequestLogger 钩子是唯一写入方，
// 一请求一行，只在请求落定（响应已写出或认证已拒）时写一次。查询只有 /api/dataset-requests。
//
// fail-loud 原则：写日志失败不静默——记 stdout，让运维能看到日志管道断了；
// 但不把错误抛回 handler（否则下游请求会因日志故障而失败，主次颠倒）。
package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// DatasetRequestLog 是 dataset_request_logs 的一行。
type DatasetRequestLog struct {
	ID           int64     `db:"id" json:"id"`
	DatasetID    string    `db:"dataset_id" json:"dataset_id"`
	Endpoint     string    `db:"endpoint" json:"endpoint"`
	ProjectID    string    `db:"project_id" json:"project_id"`
	TokenID      string    `db:"token_id" json:"token_id"`
	Store        string    `db:"store" json:"store"`
	DateFrom     string    `db:"date_from" json:"date_from"`
	DateTo       string    `db:"date_to" json:"date_to"`
	StatusCode   int       `db:"status_code" json:"status_code"`
	RowsReturned int       `db:"rows_returned" json:"rows_returned"`
	DurationMs   int64     `db:"duration_ms" json:"duration_ms"`
	ErrorMessage string    `db:"error_message" json:"error_message"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// InsertDatasetRequestLog 落一行下游请求日志。写失败只记 stdout，不影响下游请求本身。
func InsertDatasetRequestLog(db *sqlx.DB, l DatasetRequestLog) {
	const q = `
INSERT INTO dataset_request_logs
    (dataset_id, endpoint, project_id, token_id, store, date_from, date_to, status_code, rows_returned, duration_ms, error_message)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := db.Exec(q, l.DatasetID, l.Endpoint, l.ProjectID, l.TokenID, l.Store, l.DateFrom, l.DateTo, l.StatusCode, l.RowsReturned, l.DurationMs, l.ErrorMessage); err != nil {
		fmt.Printf("[db] InsertDatasetRequestLog 失败（下游请求日志丢失一行）: %v\n", err)
	}
}

// DatasetRequestLogQuery 是 /api/dataset-requests 的过滤条件。
//   - dataset / endpoint / project / status：精确匹配；空则不过滤
//   - dateFrom / dateTo：按 created_at 区间过滤；空则不收紧
type DatasetRequestLogQuery struct {
	Dataset, Endpoint, Project, Status, DateFrom, DateTo string
	Page, PageSize                                       int
}

// ListDatasetRequests 带过滤分页查询 dataset_request_logs，返回 (列表, 总数, error)。
// 排序：created_at DESC（最新在前）。
func ListDatasetRequests(db *sqlx.DB, q DatasetRequestLogQuery) ([]DatasetRequestLog, int, error) {
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	offset := (q.Page - 1) * q.PageSize

	where := make([]string, 0, 6)
	args := make([]any, 0, 6)
	if q.Dataset != "" {
		where = append(where, "dataset_id = ?")
		args = append(args, q.Dataset)
	}
	if q.Endpoint != "" {
		where = append(where, "endpoint = ?")
		args = append(args, q.Endpoint)
	}
	if q.Project != "" {
		where = append(where, "project_id = ?")
		args = append(args, q.Project)
	}
	if q.Status == "ok" {
		where = append(where, "status_code = 200")
	} else if q.Status == "error" {
		where = append(where, "status_code <> 200")
	}
	if q.DateFrom != "" {
		where = append(where, "created_at >= ?")
		args = append(args, q.DateFrom)
	}
	if q.DateTo != "" {
		where = append(where, "created_at < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, q.DateTo)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	countQ := "SELECT COUNT(*) FROM dataset_request_logs " + whereSQL
	if err := db.Get(&total, countQ, args...); err != nil {
		return nil, 0, fmt.Errorf("db.ListDatasetRequests: COUNT 失败: %w", err)
	}

	listQ := "SELECT * FROM dataset_request_logs " + whereSQL + " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	listArgs := append(args, q.PageSize, offset)
	var logs []DatasetRequestLog
	if err := db.Select(&logs, listQ, listArgs...); err != nil {
		return nil, 0, fmt.Errorf("db.ListDatasetRequests: 查列表失败: %w", err)
	}
	dbOffset, err := databaseUTCOffset(db)
	if err != nil {
		return nil, 0, err
	}
	for i := range logs {
		logs[i].CreatedAt = logs[i].CreatedAt.Add(dbOffset)
	}
	return logs, total, nil
}
