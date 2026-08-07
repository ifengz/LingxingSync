// Package db 数据访问层 —— tasks.go 系统表（sync_tasks / sync_task_logs）CRUD。
//
// 单写者原则（宪法 §8）：每个 Worker goroutine 是自己 task 行的唯一写者，
// 只有 Worker 自己 UPDATE status，不跨任务互相改。这里的方法都是薄封装，
// 不加业务锁——并发安全由「每 task_id 只一个写者」保证。
//
// fail-loud 原则：错误消息全文入库不截断（error_message TEXT），便于事后追查。
package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Task 对应 sync_tasks 一行。
//
// time 指针字段（StartedAt/FinishedAt）允许 NULL；CreatedAt 由 DB 默认值给。
type Task struct {
	ID              int64          `db:"id" json:"id"`
	Endpoint        string         `db:"endpoint" json:"endpoint"`
	AccountID       string         `db:"account_id" json:"account_id"`
	Status          string         `db:"status" json:"status"`
	TriggerType     string         `db:"trigger_type" json:"trigger_type"`
	StartedAt       *time.Time     `db:"started_at" json:"started_at"`
	FinishedAt      *time.Time     `db:"finished_at" json:"finished_at"`
	RecordsUpserted int            `db:"records_upserted" json:"records_upserted"`
	PagesFetched    int            `db:"pages_fetched" json:"pages_fetched"`
	ErrorMessage    sql.NullString `db:"error_message" json:"error_message"`
	CreatedAt       time.Time      `db:"created_at" json:"created_at"`
}

// TaskLog 对应 sync_task_logs 一行。
//
// http_status / api_code 可 NULL（请求未拿到响应时），用 sql.NullInt64。
type TaskLog struct {
	ID           int64          `db:"id" json:"id"`
	TaskID       int64          `db:"task_id" json:"task_id"`
	Page         int            `db:"page" json:"page"`
	HTTPStatus   sql.NullInt64  `db:"http_status" json:"http_status"`
	APICode      sql.NullInt64  `db:"api_code" json:"api_code"`
	RecordsCount int            `db:"records_count" json:"records_count"`
	ErrorRaw     sql.NullString `db:"error_raw" json:"error_raw"`
	DurationMs   int            `db:"duration_ms" json:"duration_ms"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
}

// 为了让 Task 的 json 序列化看着干净，ErrorMessage / TaskLog 的 NULL 字段都用 sql.Null*，
// 它们会序列化成 {Valid:bool, ...}。如果 server 层要自定义，再包一层；这里只保证落库正确。

// InsertTask 在 sync_tasks 插一行 status='running'，返回 LastInsertId。
//
// trigger_type ∈ {"cron","manual"}：区分定时触发与人工 /sync 触发。
// started_at 用 NOW()（DB 端赋值），保证多 worker 时区一致。
func InsertTask(db *sqlx.DB, endpoint, accountID, triggerType string) (int64, error) {
	if triggerType == "" {
		triggerType = "cron"
	}
	const q = `
INSERT INTO sync_tasks (endpoint, account_id, status, trigger_type, started_at)
VALUES (?, ?, 'running', ?, NOW())
`
	res, err := db.Exec(q, endpoint, accountID, triggerType)
	if err != nil {
		return 0, fmt.Errorf("db.InsertTask: 插 sync_tasks 失败 (endpoint=%s account=%s): %w",
			endpoint, accountID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("db.InsertTask: 取 LastInsertId 失败: %w", err)
	}
	return id, nil
}

// UpdateTask 由 Worker 在任务结束时调用，写终态。
//
//   - status: "success" / "error"（调用方决定）
//   - records / pages: 累计落库行数、拉取页数
//   - errMsg: nil → error_message=NULL；非 nil → 用 .Error() 全文，不截断
//
// finished_at 用 NOW()，DB 端赋值。
func UpdateTask(db *sqlx.DB, id int64, status string, records, pages int, errMsg error) error {
	var msg any
	if errMsg != nil {
		msg = errMsg.Error()
	}
	const q = `
UPDATE sync_tasks
SET finished_at = NOW(),
    status = ?,
    records_upserted = ?,
    pages_fetched = ?,
    error_message = ?
WHERE id = ?
`
	res, err := db.Exec(q, status, records, pages, msg, id)
	if err != nil {
		return fmt.Errorf("db.UpdateTask: 更新 sync_tasks id=%d 失败: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("db.UpdateTask: sync_tasks id=%d 不存在或未更新", id)
	}
	return nil
}

// CancelTask 把一个 running 任务标记为 cancelled。
//
// 仅当当前 status='running' 才更新——避免把已经 success/error 的任务改回 cancelled。
// 返回 nil 即成功；若任务已不在 running 状态，RowsAffected=0 也返回 nil（幂等取消）。
func CancelTask(db *sqlx.DB, id int64) error {
	const q = `UPDATE sync_tasks SET status = 'cancelled', finished_at = NOW() WHERE id = ? AND status = 'running'`
	if _, err := db.Exec(q, id); err != nil {
		return fmt.Errorf("db.CancelTask: 取消 sync_tasks id=%d 失败: %w", id, err)
	}
	return nil
}

// InsertTaskLog 写一条页级证据到 sync_task_logs。
//
//   - httpStatus / apiCode: 用 int 指针式参数；<=0 视为 NULL（请求未拿到响应）
//   - errRaw: 原始错误消息，空串 → NULL
//   - durationMs: 本页耗时
func InsertTaskLog(db *sqlx.DB, taskID int64, page int, httpStatus int, apiCode int, records int, durationMs int, errRaw string) error {
	var httpS, apiC any
	if httpStatus > 0 {
		httpS = httpStatus
	}
	if apiCode != 0 {
		// 领星 code=0 表示成功，负数/正数都算有效 code，只要 !=0 就记。
		apiC = apiCode
	}
	var errRawV any
	if errRaw != "" {
		errRawV = errRaw
	}
	const q = `
INSERT INTO sync_task_logs (task_id, page, http_status, api_code, records_count, error_raw, duration_ms)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
	if _, err := db.Exec(q, taskID, page, httpS, apiC, records, errRawV, durationMs); err != nil {
		return fmt.Errorf("db.InsertTaskLog: 写 sync_task_logs (task=%d page=%d) 失败: %w",
			taskID, page, err)
	}
	return nil
}

// ListTasks 带过滤分页查询 sync_tasks，返回 (列表, 总数, error)。
//
//   - endpoint: 精确匹配；空则不过滤
//   - account:  精确匹配；空则不过滤
//   - status:   精确匹配；空则不过滤
//   - dateFrom / dateTo: ISO8601 字符串，按 created_at 区间过滤；空则不收紧
//   - page 从 1 开始（page=1 → 第一页）；pageSize<=0 时给默认 20
//
// 排序：created_at DESC（最新任务在前）。
func ListTasks(db *sqlx.DB, endpoint, account, status, dateFrom, dateTo string, page, pageSize int) ([]Task, int, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	// 动态拼 WHERE，args 顺序与占位顺序一致。
	where := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if endpoint != "" {
		where = append(where, "endpoint = ?")
		args = append(args, endpoint)
	}
	if account != "" {
		where = append(where, "account_id = ?")
		args = append(args, account)
	}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if dateFrom != "" {
		where = append(where, "created_at >= ?")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "created_at < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, dateTo)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// 1. 总数
	var total int
	countQ := "SELECT COUNT(*) FROM sync_tasks " + whereSQL
	if err := db.Get(&total, countQ, args...); err != nil {
		return nil, 0, fmt.Errorf("db.ListTasks: COUNT 失败: %w", err)
	}

	// 2. 分页数据
	listQ := "SELECT * FROM sync_tasks " + whereSQL +
		" ORDER BY created_at DESC LIMIT ? OFFSET ?"
	listArgs := append(args, pageSize, offset)
	var tasks []Task
	if err := db.Select(&tasks, listQ, listArgs...); err != nil {
		return nil, 0, fmt.Errorf("db.ListTasks: 查列表失败: %w", err)
	}
	return tasks, total, nil
}

// GetTask 按 id 取单个任务。不存在返回 (nil, sql.ErrNoRows) 的包装。
func GetTask(db *sqlx.DB, id int64) (*Task, error) {
	var t Task
	const q = "SELECT * FROM sync_tasks WHERE id = ?"
	if err := db.Get(&t, q, id); err != nil {
		return nil, fmt.Errorf("db.GetTask: 查 sync_tasks id=%d 失败: %w", id, err)
	}
	return &t, nil
}

// ListTaskLogs 按 task_id 取该任务的所有页级日志，按 page 升序。
func ListTaskLogs(db *sqlx.DB, taskID int64) ([]TaskLog, error) {
	var logs []TaskLog
	const q = "SELECT * FROM sync_task_logs WHERE task_id = ? ORDER BY page ASC"
	if err := db.Select(&logs, q, taskID); err != nil {
		return nil, fmt.Errorf("db.ListTaskLogs: 查 sync_task_logs (task=%d) 失败: %w", taskID, err)
	}
	return logs, nil
}

// QuerySIDsForAccount 从 ls_stores 查某账号所有 sid（宪法 §10.3：iterate_by_store 依赖）。
//
// 返回顺序按 sid 升序，便于日志稳定。账号无店铺 → 返回空切片 + nil（调用方决定算不算错）。
func QuerySIDsForAccount(db *sqlx.DB, accountID string) ([]string, error) {
	var sids []string
	const q = "SELECT sid FROM ls_stores WHERE account_id = ? ORDER BY sid ASC"
	if err := db.Select(&sids, q, accountID); err != nil {
		return nil, fmt.Errorf("db.QuerySIDsForAccount: 查 ls_stores (account=%s) 失败: %w",
			accountID, err)
	}
	return sids, nil
}

// QueryEnabledSIDsForAccount 是 iterate_by_store 真正使用的「账号级同步闸门」。
//
// 语义（见 migrations/004）：store_sync_selection 里该账号
//   - 一行都没有  → 从未配置 → 返回 ls_stores 全部 sid（向后兼容，与 QuerySIDsForAccount 等价）；
//   - 至少有一行  → 已配置 → 只返回 enabled=1 且仍存在于 ls_stores 的 sid（INNER JOIN 天然剔除已删店铺）。
//
// 该闸门在 endpoint.StoreSids 白名单与 per-trigger storeSids 之上游：先过账号级开关，
// 再过每接口白名单，最后与手动触发交集。返回顺序按 sid 升序，与 QuerySIDsForAccount 一致。
func QueryEnabledSIDsForAccount(db *sqlx.DB, accountID string) ([]string, error) {
	var configured int
	if err := db.Get(&configured,
		"SELECT COUNT(*) FROM store_sync_selection WHERE account_id = ?", accountID); err != nil {
		return nil, fmt.Errorf("db.QueryEnabledSIDsForAccount: 查选择表行数 (account=%s) 失败: %w",
			accountID, err)
	}
	if configured == 0 {
		// 未配置：退回全放行，行为等价 QuerySIDsForAccount。
		return QuerySIDsForAccount(db, accountID)
	}
	var sids []string
	const q = `
SELECT s.sid
FROM ls_stores s
JOIN store_sync_selection sel
  ON sel.account_id = s.account_id AND sel.sid = s.sid
WHERE s.account_id = ? AND sel.enabled = 1
ORDER BY s.sid ASC`
	if err := db.Select(&sids, q, accountID); err != nil {
		return nil, fmt.Errorf("db.QueryEnabledSIDsForAccount: 查启用店铺 (account=%s) 失败: %w",
			accountID, err)
	}
	return sids, nil
}

// LoadStoreSelection 读某账号的店铺选择态，供配置页给复选框回填初值。
//
// 返回 (sid→enabled 映射, configured, error)：
//   - configured=false 表示该账号从未保存过选择（表中无行）；调用方据此决定默认勾选策略；
//   - configured=true  时，映射里没有某 sid 表示该新店铺尚未纳入上次保存 → 视作未勾选。
func LoadStoreSelection(db *sqlx.DB, accountID string) (map[string]bool, bool, error) {
	type row struct {
		SID     string `db:"sid"`
		Enabled bool   `db:"enabled"`
	}
	var rows []row
	const q = "SELECT sid, enabled FROM store_sync_selection WHERE account_id = ?"
	if err := db.Select(&rows, q, accountID); err != nil {
		return nil, false, fmt.Errorf("db.LoadStoreSelection: 查选择表 (account=%s) 失败: %w",
			accountID, err)
	}
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		m[r.SID] = r.Enabled
	}
	return m, len(rows) > 0, nil
}

// SaveStoreSelection 覆盖式保存某账号的店铺选择（HTTP handler 单独写，与单写者原则不冲突：
// store_sync_selection 不是 sync_tasks，worker 只读不写）。
//
// enabledSIDs 是「勾选参与同步」的 sid 集合；allSIDs 是配置页当前展示的该账号全部店铺 sid。
// 事务内先删该账号旧行，再对 allSIDs 每个都写一行（在 enabledSIDs 里→1，否则→0），
// 从而用「有行」表达「已配置」，用 enabled 表达勾选与否。allSIDs 为空则只清空该账号选择。
func SaveStoreSelection(db *sqlx.DB, accountID string, allSIDs, enabledSIDs []string) error {
	enabled := make(map[string]struct{}, len(enabledSIDs))
	for _, sid := range enabledSIDs {
		enabled[sid] = struct{}{}
	}

	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("db.SaveStoreSelection: 开事务 (account=%s) 失败: %w", accountID, err)
	}
	defer func() { _ = tx.Rollback() }() // 已 Commit 后 Rollback 是 no-op

	if _, err := tx.Exec("DELETE FROM store_sync_selection WHERE account_id = ?", accountID); err != nil {
		return fmt.Errorf("db.SaveStoreSelection: 清账号旧选择 (account=%s) 失败: %w", accountID, err)
	}

	if len(allSIDs) > 0 {
		placeholders := make([]string, 0, len(allSIDs))
		vals := make([]any, 0, len(allSIDs)*3)
		for _, sid := range allSIDs {
			flag := 0
			if _, ok := enabled[sid]; ok {
				flag = 1
			}
			placeholders = append(placeholders, "(?, ?, ?)")
			vals = append(vals, accountID, sid, flag)
		}
		stmt := "INSERT INTO store_sync_selection (account_id, sid, enabled) VALUES " +
			strings.Join(placeholders, ", ")
		if _, err := tx.Exec(stmt, vals...); err != nil {
			return fmt.Errorf("db.SaveStoreSelection: 写新选择 (account=%s, %d 行) 失败: %w",
				accountID, len(allSIDs), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db.SaveStoreSelection: 提交事务 (account=%s) 失败: %w", accountID, err)
	}
	return nil
}

// StoreSummary 是配置页展示的一条本地店铺摘要。SyncedAt 是本地最近写入时间，
// 不代表上游领星的实时店铺状态。
type StoreSummary struct {
	SID           string    `db:"sid" json:"sid"`
	StoreType     string    `db:"store_type" json:"store_type"`
	StoreName     string    `db:"store_name" json:"store_name"`
	SellerID      string    `db:"seller_id" json:"seller_id"`
	MarketplaceID string    `db:"marketplace_id" json:"marketplace_id"`
	Country       string    `db:"country" json:"country"`
	Status        string    `db:"status" json:"status"`
	HasAdsSetting bool      `db:"has_ads_setting" json:"has_ads_setting"`
	SyncedAt      time.Time `db:"synced_at" json:"synced_at"`
	// Enabled 不来自 ls_stores（db:"-" 让 sqlx 跳过），由 handler 结合 store_sync_selection 注解：
	// 该账号从未保存选择 → 全部 true（默认全勾）；已保存 → 仅 enabled=1 的店铺为 true。
	Enabled bool `db:"-" json:"enabled"`
}

// ListStoresForAccount 返回账号在本地 ls_stores 中的全部店铺，以及最近一条
// 本地写入时间。账号没有店铺时返回空切片和 nil，不把空数据视作错误。
func ListStoresForAccount(db *sqlx.DB, accountID string) ([]StoreSummary, *time.Time, error) {
	const q = `
SELECT sid,
       COALESCE(store_type, '') AS store_type,
       COALESCE(store_name, '') AS store_name,
       COALESCE(seller_id, '') AS seller_id,
       COALESCE(marketplace_id, '') AS marketplace_id,
       COALESCE(country, '') AS country,
       COALESCE(status, '') AS status,
       has_ads_setting,
       synced_at
FROM ls_stores
WHERE account_id = ?
ORDER BY store_name, sid`

	items := make([]StoreSummary, 0)
	if err := db.Select(&items, q, accountID); err != nil {
		return nil, nil, fmt.Errorf("db.ListStoresForAccount: 查 ls_stores (account=%s) 失败: %w", accountID, err)
	}
	var last *time.Time
	for _, item := range items {
		if last == nil || item.SyncedAt.After(*last) {
			t := item.SyncedAt
			last = &t
		}
	}
	return items, last, nil
}

// CleanupOld 按留存策略删旧记录（retention 定时任务调用）。
//
//   - taskLogsDays: sync_task_logs 按 created_at 删 N 天前
//   - tasksDays:    sync_tasks        按 created_at 删 N 天前
//
// 任一 <=0 跳过该表（不删）。失败 fail-loud。
func CleanupOld(db *sqlx.DB, taskLogsDays, tasksDays int) error {
	if taskLogsDays > 0 {
		if _, err := db.Exec(
			"DELETE FROM sync_task_logs WHERE created_at < DATE_SUB(NOW(), INTERVAL ? DAY)",
			taskLogsDays,
		); err != nil {
			return fmt.Errorf("db.CleanupOld: 清 sync_task_logs (%d 天) 失败: %w", taskLogsDays, err)
		}
	}
	if tasksDays > 0 {
		if _, err := db.Exec(
			"DELETE FROM sync_tasks WHERE created_at < DATE_SUB(NOW(), INTERVAL ? DAY) AND status IN ('success','error','cancelled')",
			tasksDays,
		); err != nil {
			return fmt.Errorf("db.CleanupOld: 清 sync_tasks (%d 天) 失败: %w", tasksDays, err)
		}
	}
	return nil
}

// TableRowCount 返回某数据表当前行数（给 /api/datasources 用）。
//
// table 来自 config，已校验；这里直接拼——表名不能用占位符。
func TableRowCount(db *sqlx.DB, table string) (int64, error) {
	var n int64
	q := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	if err := db.Get(&n, q); err != nil {
		return 0, fmt.Errorf("db.TableRowCount: COUNT %s 失败: %w", table, err)
	}
	return n, nil
}

// TableLastSync 返回某数据表最近一次 synced_at（即最后一次写入时间）。
//
// 无数据 → 返回 (nil, nil)（MAX 返回 NULL，我们映射成 nil 指针）。
func TableLastSync(db *sqlx.DB, table string) (*time.Time, error) {
	var ts sql.NullTime
	q := fmt.Sprintf("SELECT MAX(synced_at) FROM `%s`", table)
	if err := db.Get(&ts, q); err != nil {
		return nil, fmt.Errorf("db.TableLastSync: MAX(synced_at) %s 失败: %w", table, err)
	}
	if !ts.Valid {
		return nil, nil
	}
	t := ts.Time
	return &t, nil
}
