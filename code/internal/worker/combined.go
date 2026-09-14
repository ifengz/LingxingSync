package worker

import (
	"time"

	"lingxing-sync/internal/config"
)

type metricPlan struct {
	Scope  string
	Params []map[string]any
}

// metricPlansForAt keeps daily metrics and current snapshot metrics on separate
// request/write paths. A combined run still uses one worker and one task, but
// the two scopes never share the same set of writable columns.
func metricPlansForAt(endpoint config.Endpoint, metricScope string, req triggerReq, now time.Time) ([]metricPlan, error) {
	if metricScope != "combined" {
		params, err := (&EndpointWorker{Endpoint: endpoint}).paramSetsForAt(req, now)
		if err != nil {
			return nil, err
		}
		return []metricPlan{{Scope: "", Params: params}}, nil
	}

	daily, err := (&EndpointWorker{Endpoint: endpoint}).paramSetsForAt(req, now)
	if err != nil {
		return nil, err
	}

	// Snapshot values are current-state data. Even for a manual historical
	// range, the snapshot leg is intentionally anchored to today's date.
	snapshotEndpoint := endpoint
	snapshotEndpoint.WindowDays = 1
	snapshotEndpoint.DateOffsetDays = 0
	snapshotEndpoint.SingleDayWindow = true
	snapshot, err := (&EndpointWorker{Endpoint: snapshotEndpoint}).paramSetsForAt(triggerReq{kind: "cron"}, now)
	if err != nil {
		return nil, err
	}
	return []metricPlan{{Scope: "daily", Params: daily}, {Scope: "snapshot", Params: snapshot}}, nil
}

// combinedMetricColumns is deliberately narrow: only fields with proven
// semantics for the selected scope are written. Omitting the other fields from
// the INSERT keeps an upsert from replacing them with NULL.
func combinedMetricColumns(scope string, all, keys []string) []string {
	var fields []string
	switch scope {
	case "daily":
		fields = []string{"sessions", "sessions_mobile", "sessions_total", "currency_code"}
	case "snapshot":
		dailyOnly := map[string]bool{"sessions": true, "sessions_mobile": true, "sessions_total": true}
		for _, column := range all {
			if !dailyOnly[column] {
				fields = append(fields, column)
			}
		}
	default:
		return append([]string(nil), all...)
	}
	allowed := make(map[string]bool, len(all))
	for _, column := range all {
		allowed[column] = true
	}
	result := make([]string, 0, len(keys)+len(fields))
	seen := make(map[string]bool, len(keys)+len(fields))
	for _, column := range append(append([]string(nil), keys...), fields...) {
		if allowed[column] && !seen[column] {
			seen[column] = true
			result = append(result, column)
		}
	}
	return result
}
