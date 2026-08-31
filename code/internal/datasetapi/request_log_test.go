package datasetapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 下游请求日志：handler 出口必须对每次 snapshot/changes/fields 请求回调一次
// RequestLogger（成功、认证失败、参数错误都算），字段与请求一一对应。
func TestRequestLoggerCapturesSnapshotSuccessAndFailures(t *testing.T) {
	reader := &fixtureReader{page: Page{Rows: []Row{{
		Store: "store-a", Channel: "SC", ASIN: "ASIN1", SKU: "SKU1", BusinessDate: "2026-08-01",
		UpdatedAt: time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC), StableKey: "k|2026-08-01", VerificationStatus: "verified", Values: map[string]any{"units": int64(3)},
	}}}}
	h, token := newFixtureHandler(t, reader)
	var logs []RequestLog
	h.SetRequestLogger(func(l RequestLog) { logs = append(logs, l) })

	rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(logs) != 1 {
		t.Fatalf("snapshot log count=%d", len(logs))
	}
	l := logs[0]
	if l.DatasetID != DatasetID || l.Endpoint != "snapshot" || l.TokenID != "project-a" || l.ProjectID != "project-a" ||
		l.Store != "store-a" || l.DateFrom != "2026-08-01" || l.DateTo != "2026-08-01" ||
		l.StatusCode != http.StatusOK || l.RowsReturned != 1 || l.DurationMs < 0 || l.ErrorMessage != "" {
		t.Fatalf("snapshot log=%+v", l)
	}

	// 认证失败：token_id/project_id 为空，状态码 401，仍必须落一条。
	before := len(logs)
	badRec := requestJSON(t, h, http.MethodPost, SnapshotPath, "wrong-token", `{"store":"store-a"}`)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d", badRec.Code)
	}
	if len(logs) != before+1 || logs[before].Endpoint != "snapshot" || logs[before].TokenID != "" || logs[before].StatusCode != http.StatusUnauthorized || logs[before].ErrorMessage == "" {
		t.Fatalf("auth-failure log=%+v (count %d)", logs[before], len(logs))
	}

	// 参数错误：400。
	before = len(logs)
	badRec = requestJSON(t, h, http.MethodPost, SnapshotPath, token, `{"date_from":"2026-08-01"}`)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("missing store status=%d", badRec.Code)
	}
	if len(logs) != before+1 || logs[before].StatusCode != http.StatusBadRequest {
		t.Fatalf("validation-failure log=%+v", logs[before])
	}

	// changes：用首次成功 snapshot 返回的 changes cursor 请求一次，必须落一条。
	before = len(logs)
	var snapshotOut Response
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshotOut); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	if snapshotOut.Data.ChangesCursor == "" {
		t.Fatalf("snapshot returned no changes cursor: %s", rec.Body.String())
	}
	changesBody, err := json.Marshal(map[string]string{"store": "store-a", "cursor": snapshotOut.Data.ChangesCursor})
	if err != nil {
		t.Fatalf("marshal changes body: %v", err)
	}
	rec = requestJSON(t, h, http.MethodPost, ChangesPath, token, string(changesBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("changes status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(logs) != before+1 || logs[before].Endpoint != "changes" || logs[before].StatusCode != http.StatusOK || logs[before].Store != "store-a" {
		t.Fatalf("changes log=%+v", logs[before])
	}
}

// 未注入 RequestLogger 时（如部分单测环境）handler 不得因日志回调 panic。
func TestRequestLoggerOptional(t *testing.T) {
	reader := &fixtureReader{}
	h, token := newFixtureHandler(t, reader)
	rec := requestJSON(t, h, http.MethodPost, SnapshotPath, token, `{"store":"store-a","date_from":"2026-08-01","date_to":"2026-08-01"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot without logger status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// fields 路由（GET 列表 + 认证失败）也要落日志。
func TestRequestLoggerCapturesFieldsRequests(t *testing.T) {
	h, _ := newFixtureHandler(t, &fixtureReader{})
	var logs []RequestLog
	h.SetRequestLogger(func(l RequestLog) { logs = append(logs, l) })

	req := httptest.NewRequest(http.MethodGet, h.fieldsPath(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fields list status=%d", rec.Code)
	}
	if len(logs) != 1 || logs[0].Endpoint != "fields" || logs[0].StatusCode != http.StatusOK {
		t.Fatalf("fields list log=%+v", logs)
	}

	req = httptest.NewRequest(http.MethodGet, h.fieldsPath()+"?project_id=ghost", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("fields ghost project status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(logs) != 2 || logs[1].Endpoint != "fields" || logs[1].StatusCode != http.StatusForbidden || logs[1].ErrorMessage == "" {
		t.Fatalf("fields ghost log=%+v", logs[1])
	}
}
