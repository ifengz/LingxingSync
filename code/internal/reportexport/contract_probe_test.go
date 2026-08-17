package reportexport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestProbeFBAInventoryPlanningDownloadsRealTSVContract(t *testing.T) {
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/tab-separated-values; charset=utf-8")
		_, _ = w.Write([]byte("snapshot-date\tsku\tfnsku\tasin\n2026-08-17\tSKU-1\tFNSKU-1\tASIN-1\n"))
	}))
	defer download.Close()

	client := signedClientFunc(func(_ context.Context, _ string, path string, body map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			if body["report_type"] != FBAInventoryPlanningReportType {
				t.Fatalf("create report_type=%q", body["report_type"])
			}
			return []byte(`{"code":0,"data":{"task_id":"planning-task"}}`), http.StatusOK, 0, nil
		case queryPath:
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"planning-document","url":"%s"}}`, download.URL)), http.StatusOK, 0, nil
		default:
			t.Fatalf("unexpected signed path %s", path)
			return nil, 0, 0, nil
		}
	})

	result, err := ProbeFBAInventoryPlanning(context.Background(), client, Request{
		ReportType:     FBAInventoryPlanningReportType,
		AccountID:      "sc_us_1",
		SellerID:       "seller-1",
		StoreID:        "store-1",
		Region:         "na",
		MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		DateFrom:       "2026-08-17T00:00:00Z",
		DateTo:         "2026-08-17T23:59:59Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ReportTaskID != "planning-task" || result.ReportDocumentID != "planning-document" {
		t.Fatalf("probe identifiers=%#v", result)
	}
	if result.Rows != 1 || !slices.Equal(result.Header, []string{"snapshot-date", "sku", "fnsku", "asin"}) {
		t.Fatalf("probe contract rows=%d header=%#v", result.Rows, result.Header)
	}
	if result.DownloadSHA256 == "" || result.ContentType != "text/tab-separated-values; charset=utf-8" {
		t.Fatalf("probe metadata=%#v", result)
	}
}
