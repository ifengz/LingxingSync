package reportexport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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

	limiter := &countingLimiter{}
	result, err := ProbeFBAInventoryPlanning(context.Background(), client, limiter, Request{
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
	if limiter.waits != 2 {
		t.Fatalf("limiter waits=%d, want create and query", limiter.waits)
	}
}

func TestProbeFBAInventoryPlanningRequiresLimiter(t *testing.T) {
	called := false
	client := signedClientFunc(func(context.Context, string, string, map[string]any) ([]byte, int, int, error) {
		called = true
		return nil, 0, 0, nil
	})
	_, err := ProbeFBAInventoryPlanning(context.Background(), client, nil, Request{
		ReportType:     FBAInventoryPlanningReportType,
		AccountID:      "sc_us_1",
		SellerID:       "seller-1",
		StoreID:        "store-1",
		Region:         "na",
		MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		DateFrom:       "2026-08-17T00:00:00Z",
		DateTo:         "2026-08-17T23:59:59Z",
	})
	if err == nil || !strings.Contains(err.Error(), "limiter is required") {
		t.Fatalf("error=%v, want missing limiter rejection", err)
	}
	if called {
		t.Fatal("probe called Lingxing without a limiter")
	}
}

func TestProbeFBAInventoryPlanningDoesNotExposeSignedDownloadURL(t *testing.T) {
	const signature = "probe-signature-must-not-leak"
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			return []byte(`{"code":0,"data":{"task_id":"planning-task"}}`), http.StatusOK, 0, nil
		case queryPath:
			return []byte(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"planning-document","url":"https://example.invalid/%zz?X-Amz-Signature=` + signature + `"}}`), http.StatusOK, 0, nil
		case renewPath:
			return nil, http.StatusInternalServerError, 0, fmt.Errorf("renew failed")
		default:
			t.Fatalf("unexpected signed path %s", path)
			return nil, 0, 0, nil
		}
	})
	_, err := ProbeFBAInventoryPlanning(context.Background(), client, &countingLimiter{}, Request{
		ReportType:     FBAInventoryPlanningReportType,
		AccountID:      "sc_us_1",
		SellerID:       "seller-1",
		StoreID:        "store-1",
		Region:         "na",
		MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		DateFrom:       "2026-08-17T00:00:00Z",
		DateTo:         "2026-08-17T23:59:59Z",
	})
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("error=%v, want fixed download failure", err)
	}
	if strings.Contains(err.Error(), signature) {
		t.Fatal("probe download error exposed signed URL")
	}
}

func TestProbeFBAInventoryPlanningDoesNotExposeSignedQueryDiagnostics(t *testing.T) {
	const signature = "query-signature-must-not-leak"
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			return []byte(`{"code":0,"data":{"task_id":"planning-task"}}`), http.StatusOK, 0, nil
		case queryPath:
			return []byte(`{"code":0,"message":"https://example.invalid/file?X-Amz-Signature=` + signature + `","data":{"progress_status":"FATAL"}}`), http.StatusOK, 0, nil
		default:
			t.Fatalf("unexpected signed path %s", path)
			return nil, 0, 0, nil
		}
	})
	_, err := ProbeFBAInventoryPlanning(context.Background(), client, &countingLimiter{}, Request{
		ReportType:     FBAInventoryPlanningReportType,
		AccountID:      "sc_us_1",
		SellerID:       "seller-1",
		StoreID:        "store-1",
		Region:         "na",
		MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		DateFrom:       "2026-08-17T00:00:00Z",
		DateTo:         "2026-08-17T23:59:59Z",
	})
	if err == nil {
		t.Fatal("probe query should return FATAL")
	}
	if strings.Contains(err.Error(), signature) {
		t.Fatal("probe query diagnostics exposed signed URL")
	}
}

func TestReadProbeTSVRejectsNoBusinessRows(t *testing.T) {
	_, _, err := readProbeTSV([]byte("snapshot-date\tsku\n"), "", "text/tab-separated-values; charset=utf-8")
	if err == nil || !strings.Contains(err.Error(), "has no business rows") {
		t.Fatalf("error=%v, want zero-row stop", err)
	}
}

func TestReadProbeTSVRejectsOnlyEmptyRows(t *testing.T) {
	_, _, err := readProbeTSV([]byte("snapshot-date\tsku\n\t\n"), "", "text/tab-separated-values; charset=utf-8")
	if err == nil || !strings.Contains(err.Error(), "has no business rows") {
		t.Fatalf("error=%v, want empty-row stop", err)
	}
}

func TestReadProbeTSVReportsPhysicalRowAfterEmptyRows(t *testing.T) {
	_, _, err := readProbeTSV([]byte("snapshot-date\tsku\n\t\n\"unterminated"), "", "text/tab-separated-values; charset=utf-8")
	if err == nil || !strings.Contains(err.Error(), "row 3") {
		t.Fatalf("error=%v, want physical row 3", err)
	}
}
