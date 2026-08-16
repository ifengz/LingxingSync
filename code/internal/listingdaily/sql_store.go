package listingdaily

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

// SQLStore is the concrete, transaction-scoped publisher for listing daily facts.
// It never reads or mutates ls_* raw evidence tables.
type SQLStore struct{ DB *sqlx.DB }

var _ ReconciliationStore = SQLStore{}

func (s SQLStore) Persist(ctx context.Context, rows []Metric) error {
	return s.persistBatch(ctx, rows, nil)
}

func (s SQLStore) PersistReportBatch(ctx context.Context, rows []Metric, audits []ReconciliationAudit) error {
	return s.persistBatch(ctx, rows, audits)
}

func (s SQLStore) PersistFailedReconciliations(ctx context.Context, audits []ReconciliationAudit) error {
	if s.DB == nil {
		return fmt.Errorf("listing daily: nil database")
	}
	for _, audit := range audits {
		if audit.Status != ReconciliationFailed {
			return fmt.Errorf("listing daily: failure audit has status %q", audit.Status)
		}
		if err := persistReconciliation(ctx, s.DB, audit); err != nil {
			return err
		}
	}
	return nil
}

func (s SQLStore) persistBatch(ctx context.Context, rows []Metric, audits []ReconciliationAudit) error {
	if s.DB == nil {
		return fmt.Errorf("listing daily: nil database")
	}
	if len(rows) == 0 && len(audits) == 0 {
		return nil
	}
	rows = metricsForPersistence(rows)
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing daily: begin publish transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := persistMetrics(ctx, tx, rows); err != nil {
		return err
	}
	for _, audit := range audits {
		if err := persistReconciliation(ctx, tx, audit); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("listing daily: commit publish transaction: %w", err)
	}
	return nil
}

func metricsForPersistence(rows []Metric) []Metric {
	ordered := append([]Metric(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return keyID(ordered[i].Key, ordered[i].Scope) < keyID(ordered[j].Key, ordered[j].Scope)
	})
	return ordered
}

func persistMetrics(ctx context.Context, tx *sqlx.Tx, rows []Metric) error {
	for _, row := range rows {
		if err := validateInput(Input{Key: row.Key, Scope: row.Scope, Values: row.Values}); err != nil {
			return err
		}
		row.Key = normalizedKey(row.Key)
		dimensionID, err := upsertDimension(ctx, tx, row)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, metricsUpsertSQL, metricArgs(dimensionID, row)...); err != nil {
			return fmt.Errorf("listing daily: publish %s: %w", keyID(row.Key, row.Scope), err)
		}
	}
	return nil
}

const reconciliationUpsertSQL = `INSERT INTO listing_daily_reconciliations
(report_audit_id, report_task_id, business_date, status, missing_in_db, missing_in_report, field_diffs, error_message)
VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
ON DUPLICATE KEY UPDATE
report_task_id = VALUES(report_task_id), status = VALUES(status),
missing_in_db = VALUES(missing_in_db), missing_in_report = VALUES(missing_in_report),
field_diffs = VALUES(field_diffs), error_message = VALUES(error_message), updated_at = CURRENT_TIMESTAMP(6)`

type reconciliationExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func persistReconciliation(ctx context.Context, execer reconciliationExecer, audit ReconciliationAudit) error {
	if audit.Evidence.AuditID <= 0 || strings.TrimSpace(audit.Evidence.ReportTaskID) == "" || audit.BusinessDate.IsZero() {
		return fmt.Errorf("listing daily: reconciliation audit requires report audit, task, and business date")
	}
	if audit.Status != ReconciliationMatched && audit.Status != ReconciliationCorrected && audit.Status != ReconciliationFailed {
		return fmt.Errorf("listing daily: invalid reconciliation status %q", audit.Status)
	}
	missingInDB, err := json.Marshal(audit.Reconciliation.MissingInDB)
	if err != nil {
		return fmt.Errorf("listing daily: encode missing_in_db: %w", err)
	}
	missingInReport, err := json.Marshal(audit.Reconciliation.MissingInReport)
	if err != nil {
		return fmt.Errorf("listing daily: encode missing_in_report: %w", err)
	}
	fieldDiffs, err := json.Marshal(audit.Reconciliation.FieldDiffs)
	if err != nil {
		return fmt.Errorf("listing daily: encode field_diffs: %w", err)
	}
	_, err = execer.ExecContext(ctx, reconciliationUpsertSQL,
		audit.Evidence.AuditID, audit.Evidence.ReportTaskID, calendarDate(audit.BusinessDate), audit.Status,
		missingInDB, missingInReport, fieldDiffs, audit.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("listing daily: persist reconciliation audit: %w", err)
	}
	return nil
}

func upsertDimension(ctx context.Context, tx *sqlx.Tx, row Metric) (int64, error) {
	key := normalizedKey(row.Key)
	identityKey := key.ASIN + "\x00" + key.SKU
	if row.Scope == ScopeStore {
		identityKey = key.Store
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO listing_dimensions
(store_id, channel, identity_scope, identity_key, asin, sku)
VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))
ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), asin = VALUES(asin), sku = VALUES(sku)`,
		key.Store, key.Channel, row.Scope, identityKey, key.ASIN, key.SKU)
	if err != nil {
		return 0, fmt.Errorf("listing daily: upsert dimension %s: %w", keyID(key, row.Scope), err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("listing daily: read dimension id: %w", err)
	}
	if id == 0 {
		return 0, fmt.Errorf("listing daily: dimension id is zero for %s", keyID(key, row.Scope))
	}
	return id, nil
}

var metricsUpsertSQL = buildMetricsUpsertSQL()

func buildMetricsUpsertSQL() string {
	fields := knownMetricFieldNames()
	columns := []string{"listing_dimension_id", "business_date"}
	values := []string{"?", "?"}
	updates := make([]string, 0, len(fields)*2+5)
	for _, field := range fields {
		columns = append(columns, field, field+"_source")
		values = append(values, "?", "?")
		condition := "(VALUES(" + field + "_source) = 'report' OR (VALUES(" + field + "_source) = 'api' AND " + field + "_source <> 'report'))"
		updates = append(updates,
			field+" = IF("+condition+", VALUES("+field+"), "+field+")",
			field+"_source = IF("+condition+", VALUES("+field+"_source), "+field+"_source)",
		)
	}
	columns = append(columns, "is_provisional", "is_verified", "verified_fields", "report_verified_at")
	values = append(values, "?", "?", "?", "IF(?, CURRENT_TIMESTAMP, NULL)")
	updates = append(updates,
		"is_provisional = IF(VALUES(is_verified), FALSE, VALUES(is_provisional))",
		"is_verified = GREATEST(is_verified, VALUES(is_verified))",
		"verified_fields = JSON_MERGE_PATCH(verified_fields, VALUES(verified_fields))",
		"report_verified_at = IF(VALUES(is_verified), CURRENT_TIMESTAMP, report_verified_at)",
	)
	return "INSERT INTO listing_daily_metrics (\n" + strings.Join(columns, ", ") + "\n) VALUES (\n" + strings.Join(values, ", ") + "\n) ON DUPLICATE KEY UPDATE\n" + strings.Join(updates, ",\n")
}

func metricArgs(dimensionID int64, row Metric) []any {
	args := []any{dimensionID, row.Key.BusinessDate}
	for _, field := range knownMetricFieldNames() {
		args = append(args, metricField(row.Values, field), row.Sources[field])
	}
	return append(args, row.IsProvisional, row.IsVerified, verifiedFieldsJSON(row), row.IsVerified)
}

func verifiedFieldsJSON(row Metric) string {
	fields := row.VerifiedFields
	if fields == nil {
		fields = map[string]bool{}
	}
	b, _ := json.Marshal(fields)
	return string(b)
}
