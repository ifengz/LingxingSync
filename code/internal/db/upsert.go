// Package db 数据访问层 —— upsert.go 实现宪法 §9 的通用 Upsert。
//
// 这是「零代码加接口」的核心：只要 migrations 把 ls_xxx 表建好，
// worker 拿到领星返回的 []map[string]any（字段名 → 值），调 UpsertRows 即可落库，
// 不需要为每个接口写专用 INSERT。
//
// 字段规则（宪法 §9）：
//   - account_id 由本系统注入，不在领星返回里
//   - synced_at 只使用 MySQL 当前时间，绝不接受上游值
//   - 主键 = (account_id, 业务唯一键)；冲突时 ON DUPLICATE KEY UPDATE 覆盖非主键列
//   - account_id 不进 UPDATE 子句（它是主键的一部分，UPDATE 它没意义且会破坏索引）
package db

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// GetTableColumns 查询某表所有列名（按建表顺序）。
//
// 用于启动断言：如果表不存在（返回空切片），调用方应 fail-loud，不静默兜底。
// 宪法 §9：worker 启动时会调它验证 endpoint.Table 真实存在且列与配置匹配。
func GetTableColumns(db *sqlx.DB, table string) ([]string, error) {
	const q = `
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION
`
	var cols []string
	if err := db.Select(&cols, q, table); err != nil {
		return nil, fmt.Errorf("db.GetTableColumns: 查 %s 列信息失败: %w", table, err)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("db.GetTableColumns: 表 %s 不存在或无列（TABLE_SCHEMA=%s）",
			table, currentDB(db))
	}
	return cols, nil
}

// GetJSONColumns returns the JSON-typed columns for a table. The worker caches
// this once at startup so JSON-specific input handling stays column-aware.
func GetJSONColumns(db *sqlx.DB, table string) (map[string]bool, error) {
	const q = `
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND DATA_TYPE = 'json'
`
	var cols []string
	if err := db.Select(&cols, q, table); err != nil {
		return nil, fmt.Errorf("db.GetJSONColumns: 查 %s JSON 列失败: %w", table, err)
	}
	result := make(map[string]bool, len(cols))
	for _, col := range cols {
		result[col] = true
	}
	return result, nil
}

// currentDB 尽力返回当前库名，仅用于错误信息，失败不影响主流程。
func currentDB(db *sqlx.DB) string {
	var name string
	if err := db.Get(&name, "SELECT DATABASE()"); err != nil {
		return "?"
	}
	return name
}

// UpsertRows 把领星返回的若干行批量写入库。
//
// 参数：
//   - table: 目标表名（如 "ls_sales_orders"）
//   - rows:  领星返回的行，每行 map[string]any，key 是字段名（不含 account_id / synced_at）
//   - allowedCols: 该接口允许写入的列名集合（来自 config + 表结构交集，含 account_id 无妨）
//   - accountID: 本系统内部账号 ID，注入到每行 account_id 列
//
// 行为：
//   - rows 空 → 直接返回 nil（不是错误）
//   - 列集合 = ["account_id"] + (allowedCols 去掉 account_id 和 synced_at)，保持稳定顺序
//   - 生成 INSERT ... VALUES (...),(...) ... ON DUPLICATE KEY UPDATE 非主键列=VALUES(列)
//   - 每行字段缺失 → 写 nil → SQL NULL
//   - 单批失败立即返回 error（fail-loud：类型不兼容绝不能写进脏数据）
func UpsertRows(db *sqlx.DB, table string, rows []map[string]any, allowedCols []string, jsonCols map[string]bool, accountID string) error {
	if len(rows) == 0 {
		return nil
	}

	// 1. 计算最终列集合：account_id 在最前，去掉 synced_at（由 DB 管理），去掉重复的 account_id。
	cols := buildUpsertColumns(allowedCols)
	if len(cols) <= 1 {
		// 只有 account_id 一列，说明 allowedCols 里没有任何业务列——这是配置/表结构错误。
		return fmt.Errorf("db.UpsertRows: 表 %s 没有可写的业务列（allowedCols=%v）", table, allowedCols)
	}

	// 2. 构造 SQL。只 touch 实际拥有 synced_at 的 raw 表；这样完整快照
	// 中本次仍返回但数值未变的行也属于今天，未再返回的旧行不会被触碰。
	stmt := fmt.Sprintf("INSERT INTO `%s` %s", table, buildUpsertStatement(cols, shouldTouchSnapshot(table, allowedCols), len(rows)))

	// 3. 按 [accountID, row[c0], row[c1], ...] 顺序铺 vals。
	//    缺失字段写 nil（→ SQL NULL）。
	vals := make([]any, 0, len(rows)*len(cols))
	bizCols := cols[1:] // 对应每行 accountID 之后的部分
	for _, row := range rows {
		vals = append(vals, accountID)
		for _, c := range bizCols {
			v, ok := row[c]
			if !ok {
				vals = append(vals, nil)
				continue
			}
			// 领星偶尔把数字字符串化，这里不强行转型——交给 driver 处理；
			// 类型不兼容时 Exec 会报错，符合 fail-loud。
			// 例外：slice/map（领星 JSON 字段，如 afn_fulfillable_quantity_multi）
			// 须先 JSON 序列化为字符串，driver 才能写入 JSON 列，否则报 unsupported type。
			vals = append(vals, normalizeUpsertValue(v, jsonCols[c]))
		}
	}

	if _, err := db.Exec(stmt, vals...); err != nil {
		return fmt.Errorf("db.UpsertRows: 写表 %s 失败（%d 行，%d 列）: %w",
			table, len(rows), len(cols), err)
	}
	return nil
}

func containsColumn(columns []string, target string) bool {
	for _, column := range columns {
		if column == target {
			return true
		}
	}
	return false
}

func shouldTouchSnapshot(table string, columns []string) bool {
	return table == "ls_fba_inventory" && containsColumn(columns, "synced_at")
}

func buildUpsertStatement(cols []string, touchSyncedAt bool, rowCount int) string {
	placeRow := "(" + strings.Repeat("?,", len(cols)-1) + "?)"
	valuePlaceholders := strings.Repeat(placeRow+",", rowCount-1) + placeRow
	updates := make([]string, 0, len(cols))
	for _, column := range cols[1:] {
		updates = append(updates, fmt.Sprintf("`%s` = VALUES(`%s`)", column, column))
	}
	if touchSyncedAt {
		updates = append(updates, "`synced_at` = CURRENT_TIMESTAMP")
	}
	quotedCols := make([]string, len(cols))
	for i, column := range cols {
		quotedCols[i] = "`" + column + "`"
	}
	return fmt.Sprintf("(%s) VALUES %s ON DUPLICATE KEY UPDATE %s", strings.Join(quotedCols, ", "), valuePlaceholders, strings.Join(updates, ", "))
}

// normalizeUpsertValue 处理领星返回值到 driver 可写入的形态。
//
// 唯一转换：slice/map（领星的 JSON 数组/对象字段，如多站点可售量
// afn_fulfillable_quantity_multi）先 JSON 序列化为字符串，否则 driver 报
// "unsupported type"、无法写入 MySQL JSON 列。
//
// JSON 列的空字符串表示上游没有值，写成 SQL NULL；其他一切（数字、字符串、bool、nil）原样透传——不改名、不转型，
// 类型不兼容时交给 Exec fail-loud（宪法：不静默兜底、不写脏数据）。
func normalizeUpsertValue(v any, jsonColumn bool) any {
	if jsonColumn {
		if s, ok := v.(string); ok && s == "" {
			return nil
		}
	}
	switch v.(type) {
	case []any, map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			// 序列化失败极罕见（领星 JSON 已能 unmarshal 成这些类型）。
			return nil
		}
		return string(b)
	default:
		return v
	}
}

// buildUpsertColumns 把 allowedCols 规整成最终写入列：
//   - 第一位永远是 "account_id"
//   - 去掉同步框架自管的时间列（DB 自管）
//   - 去掉后续重复出现的 "account_id"
//   - 保持 allowedCols 原始顺序（INSERT 列顺序稳定，便于排错）
func buildUpsertColumns(allowedCols []string) []string {
	cols := make([]string, 0, len(allowedCols)+1)
	cols = append(cols, "account_id")
	seen := map[string]bool{"account_id": true}
	for _, c := range allowedCols {
		if c == "" {
			continue
		}
		if c == "synced_at" || c == "last_attempt_at" || c == "last_success_at" {
			continue
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		cols = append(cols, c)
	}
	return cols
}
