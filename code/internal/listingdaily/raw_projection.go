package listingdaily

import (
	"context"
	"fmt"
	"time"
)

// RawRecord is the explicit boundary from independently retained ls_* raw rows
// (or a report raw row) into this derived fact package. It contains no DB handle
// and never mutates the raw evidence it was projected from.
type RawRecord struct {
	Source Source
	Input  Input
}

// ProjectRaw validates the evidence boundary before a record can enter the fact
// set. Snapshot-only sources must leave unsupported daily fields nil upstream.
func ProjectRaw(records []RawRecord, source Source) ([]Input, error) {
	if source != SourceAPI && source != SourceReport {
		return nil, fmt.Errorf("listing daily: unsupported raw source %q", source)
	}
	inputs := make([]Input, len(records))
	for i, record := range records {
		if record.Source != source {
			return nil, fmt.Errorf("listing daily: raw record %d source %q does not match %q", i, record.Source, source)
		}
		if err := validateInput(record.Input); err != nil {
			return nil, fmt.Errorf("listing daily: raw record %d: %w", i, err)
		}
		inputs[i] = record.Input
	}
	return inputs, nil
}

// ProjectAndPublish is the concrete call path: project API/report raw evidence,
// merge only after report reconciliation, then commit the fact batch atomically.
func ProjectAndPublish(ctx context.Context, store Store, apiRaw, reportRaw []RawRecord, reportState ReportState, today time.Time) error {
	if store == nil {
		return fmt.Errorf("listing daily: nil store")
	}
	rows, err := Build(apiRaw, reportRaw, reportState, today)
	if err != nil {
		return err
	}
	return store.Persist(ctx, rows)
}

// Build validates and assembles one target without writing it. Callers can
// build every store/date first, then commit the combined rows once.
func Build(apiRaw, reportRaw []RawRecord, reportState ReportState, today time.Time) ([]Metric, error) {
	api, err := ProjectRaw(apiRaw, SourceAPI)
	if err != nil {
		return nil, err
	}
	report, err := ProjectRaw(reportRaw, SourceReport)
	if err != nil {
		return nil, err
	}
	if reportState == ReportFailed {
		return nil, ErrReportReconciliationFailed
	}
	rows, err := Assemble(api, report, reportState == ReportReconciled)
	if err != nil {
		return nil, err
	}
	return Prepare(Batch{Rows: rows, ReportState: reportState}, today)
}
