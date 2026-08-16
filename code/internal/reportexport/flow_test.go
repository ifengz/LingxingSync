package reportexport

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"lingxing-sync/internal/api"
)

type signedClientFunc func(context.Context, string, string, map[string]any) ([]byte, int, int, error)

func (f signedClientFunc) DoSignedJSON(ctx context.Context, method, path string, body map[string]any) ([]byte, int, int, error) {
	return f(ctx, method, path, body)
}

func TestParseCustomerReturnsGZIPTSVRequiresExactHeader(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write([]byte("return-date\torder-id\tsku\tasin\tfnsku\tproduct-name\tquantity\tfulfillment-center-id\tdetailed-disposition\treason\tstatus\tlicense-plate-number\tcustomer-comments\n2026-08-11\torder-1\tsku-1\tasin-1\tfnsku-1\tWidget\t2\tFC1\tSELLABLE\tOTHER\tCOMPLETE\tlp-1\tok\n"))
	_ = zw.Close()
	rows, err := ParseCustomerReturns(compressed.Bytes(), "GZIP", "")
	if err != nil {
		t.Fatalf("ParseCustomerReturns returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Quantity != 2 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseCustomerShipmentSalesTSVRequiresOfficialHeaderAndNumericFields(t *testing.T) {
	data := []byte("shipment-date\tsku\tfnsku\tasin\tfulfillment-center-id\tquantity\tamazon-order-id\tcurrency\titem-price-per-unit\tshipping-price\tgift-wrap-price\tship-city\tship-state\tship-postal-code\n2026-08-11\tsku-1\tfnsku-1\tasin-1\tFC1\t3\torder-1\tUSD\t12.50\t1.25\t0.00\tSeattle\tWA\t98101\n")
	rows, err := ParseCustomerShipmentSales(data, "", "")
	if err != nil {
		t.Fatalf("ParseCustomerShipmentSales returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Quantity != 3 || rows[0].ItemPricePerUnit != 12.5 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestParseCustomerReturnsDecodesDeclaredCP1252(t *testing.T) {
	data := []byte("return-date\torder-id\tsku\tasin\tfnsku\tproduct-name\tquantity\tfulfillment-center-id\tdetailed-disposition\treason\tstatus\tlicense-plate-number\tcustomer-comments\n2026-08-08\torder-1\tsku-1\tasin-1\tfnsku-1\tWidget\t1\tFC1\tSELLABLE\tOTHER\tCOMPLETE\tlp-1\t\x93opened\x94\n")
	rows, err := ParseCustomerReturns(data, "", "text/plain; charset=cp1252")
	if err != nil {
		t.Fatalf("ParseCustomerReturns returned error: %v", err)
	}
	if got := rows[0].CustomerComments; got != "\u201copened\u201d" {
		t.Fatalf("customer comments = %q, want decoded cp1252 punctuation", got)
	}
}

func TestParseCustomerReturnsFailsOnMissingHeaderColumn(t *testing.T) {
	_, err := ParseCustomerReturns([]byte("return-date\torder-id\n"), "", "")
	if err == nil {
		t.Fatal("expected exact-header error")
	}
}

func TestParseCustomerReturnsRejectsUnknownCompression(t *testing.T) {
	_, err := ParseCustomerReturns([]byte("not a zip file"), "ZIP", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported compression") {
		t.Fatalf("error = %v, want unsupported compression", err)
	}
}

type fakeStore struct {
	nextID                int64
	audits                []Audit
	progress              []string
	saved                 int
	savedFBA              int
	savedFBAAll           int
	savedReserved         int
	savedAFN              int
	savedAFNByCountry     int
	savedStorageFees      int
	savedOverageFees      int
	savedLongtermFees     int
	savedReplacements     int
	savedReimbursements   int
	savedStranded         int
	savedEstimatedFees    int
	savedNoncompliance    int
	savedRecommended      int
	savedRemovalOrders    int
	savedRemovalShipments int
	savedFulfilled        int
	errors                int
	markErrorErr          error
}

type doneAuditStore struct{ fakeStore }

func (s *doneAuditStore) EnsureReport(context.Context, Request) (Audit, error) {
	return Audit{ID: 1, ReportTaskID: "done-task", Status: "DONE", CreateClaimed: false}, nil
}

func TestRunnerMarksSharedDoneParseFailureAsTerminalError(t *testing.T) {
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("wrong\theader\n"))
	}))
	defer download.Close()
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		if path != queryPath {
			return nil, 0, 0, fmt.Errorf("unexpected signed path %s", path)
		}
		return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"doc-1","url":"%s/file"}}`, download.URL)), 200, 0, nil
	})
	store := &doneAuditStore{}
	runner := Runner{Client: client, Store: store, PollTimeout: time.Second}
	_, err := runner.Run(context.Background(), Request{ReportType: AllOrdersReportType, AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"market"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
	if err == nil || store.errors != 1 {
		t.Fatalf("err=%v mark_errors=%d, want parse failure and one terminal mark", err, store.errors)
	}
}

type countingLimiter struct{ waits int }

func (l *countingLimiter) Wait(context.Context) error { l.waits++; return nil }

func (s *fakeStore) EnsureReport(context.Context, Request) (Audit, error) {
	s.nextID++
	audit := Audit{ID: s.nextID, Status: "CREATING", CreateClaimed: true}
	s.audits = append(s.audits, audit)
	return audit, nil
}
func (s *fakeStore) LoadReport(_ context.Context, id int64) (Audit, error) {
	for _, audit := range s.audits {
		if audit.ID == id {
			return audit, nil
		}
	}
	return Audit{}, fmt.Errorf("missing audit %d", id)
}
func (s *fakeStore) MarkReportCreated(_ context.Context, id int64, taskID string) error {
	for i := range s.audits {
		if s.audits[i].ID == id {
			s.audits[i].ReportTaskID = taskID
		}
	}
	return nil
}
func (s *fakeStore) MarkReportProgress(_ context.Context, _ int64, status, _, _, _ string) error {
	s.progress = append(s.progress, status)
	return nil
}
func (s *fakeStore) SaveCustomerReturns(_ context.Context, _ int64, rows []CustomerReturn, _ string, _ string) error {
	s.saved += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAInventory(_ context.Context, _ int64, rows []FBAInventory, _ string, _ string) error {
	s.savedFBA += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAAllInventory(_ context.Context, _ int64, rows []FBAAllInventory, _ string, _ string) error {
	s.savedFBAAll += len(rows)
	return nil
}
func (s *fakeStore) SaveReservedInventory(_ context.Context, _ int64, rows []ReservedInventory, _ string, _ string) error {
	s.savedReserved += len(rows)
	return nil
}
func (s *fakeStore) SaveAFNInventory(_ context.Context, _ int64, rows []AFNInventory, _ string, _ string) error {
	s.savedAFN += len(rows)
	return nil
}
func (s *fakeStore) SaveAFNInventoryByCountry(_ context.Context, _ int64, rows []AFNInventoryByCountry, _ string, _ string) error {
	s.savedAFNByCountry += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAStorageFeeCharges(_ context.Context, _ int64, rows []FBAStorageFeeCharges, _ string, _ string) error {
	s.savedStorageFees += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAOverageFeeCharges(_ context.Context, _ int64, rows []FBAOverageFeeCharges, _ string, _ string) error {
	s.savedOverageFees += len(rows)
	return nil
}
func (s *fakeStore) SaveFBALongtermStorageFeeCharges(_ context.Context, _ int64, rows []FBALongtermStorageFeeCharges, _ string, _ string) error {
	s.savedLongtermFees += len(rows)
	return nil
}
func (s *fakeStore) SaveCustomerShipmentReplacements(_ context.Context, _ int64, rows []CustomerShipmentReplacement, _ string, _ string) error {
	s.savedReplacements += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAReimbursements(_ context.Context, _ int64, rows []FBAReimbursement, _ string, _ string) error {
	s.savedReimbursements += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAStrandedInventory(_ context.Context, _ int64, rows []FBAStrandedInventory, _ string, _ string) error {
	s.savedStranded += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAEstimatedFees(_ context.Context, _ int64, rows []FBAEstimatedFees, _ string, _ string) error {
	s.savedEstimatedFees += len(rows)
	return nil
}
func (s *fakeStore) SaveFBAInboundNoncompliance(_ context.Context, _ int64, rows []FBAInboundNoncompliance, _ string, _ string) error {
	s.savedNoncompliance += len(rows)
	return nil
}
func (s *fakeStore) SaveFBARecommendedRemoval(_ context.Context, _ int64, rows []FBARecommendedRemoval, _ string, _ string) error {
	s.savedRecommended += len(rows)
	return nil
}
func (s *fakeStore) SaveFBARemovalOrder(_ context.Context, _ int64, rows []FBARemovalOrder, _ string, _ string) error {
	s.savedRemovalOrders += len(rows)
	return nil
}
func (s *fakeStore) SaveFBARemovalShipment(_ context.Context, _ int64, rows []FBARemovalShipment, _ string, _ string) error {
	s.savedRemovalShipments += len(rows)
	return nil
}
func (s *fakeStore) SaveFulfilledShipments(_ context.Context, _ int64, rows []FulfilledShipment, _ string, _ string) error {
	s.savedFulfilled += len(rows)
	return nil
}
func (s *fakeStore) MarkReportError(_ context.Context, _ int64, _ string, _ error) error {
	s.errors++
	return s.markErrorErr
}

type salesStore struct {
	fakeStore
	savedSales int
}

func (s *salesStore) SaveCustomerShipmentSales(_ context.Context, _ int64, rows []CustomerShipmentSale, _ string, _ string) error {
	s.savedSales += len(rows)
	return nil
}

func TestRunnerExportsCustomerShipmentSalesAndPersistsTypedRows(t *testing.T) {
	data := []byte("shipment-date\tsku\tfnsku\tasin\tfulfillment-center-id\tquantity\tamazon-order-id\tcurrency\titem-price-per-unit\tshipping-price\tgift-wrap-price\tship-city\tship-state\tship-postal-code\n2026-08-11\tsku-1\tfnsku-1\tasin-1\tFC1\t3\torder-1\tUSD\t12.50\t1.25\t0.00\tSeattle\tWA\t98101\n")
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer download.Close()
	client := signedClientFunc(func(_ context.Context, _ string, path string, body map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			if body["report_type"] != CustomerShipmentSalesReportType {
				t.Fatalf("create report_type = %#v", body["report_type"])
			}
			return []byte(`{"code":0,"data":{"task_id":"sales-task"}}`), 200, 0, nil
		case queryPath:
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"sales-doc","url":"%s/file"}}`, download.URL)), 200, 0, nil
		default:
			return nil, 0, 0, fmt.Errorf("unexpected signed path %s", path)
		}
	})
	store := &salesStore{}
	runner := Runner{Client: client, Store: store, PollInterval: time.Millisecond, PollTimeout: time.Second}
	result, err := runner.Run(context.Background(), Request{ReportType: CustomerShipmentSalesReportType, AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
	if err != nil || result.Status != "SUCCESS" {
		t.Fatalf("result=%#v, err=%v", result, err)
	}
	if result.Rows != 1 || store.savedSales != 1 {
		t.Fatalf("result rows=%d saved sales=%d", result.Rows, store.savedSales)
	}
}

func TestRunnerPersistsEachInventoryReportThroughItsTypedStore(t *testing.T) {
	fixtures := map[string][]byte{
		FBAInventoryReportType:                 []byte("sku\tfnsku\tasin\tproduct-name\tcondition\tyour-price\tmfn-listing-exists\tmfn-fulfillable-quantity\tafn-listing-exists\tafn-warehouse-quantity\tafn-fulfillable-quantity\tafn-unsellable-quantity\tafn-reserved-quantity\tafn-total-quantity\tper-unit-volume\tafn-inbound-working-quantity\tafn-inbound-shipped-quantity\tafn-inbound-receiving-quantity\tafn-researching-quantity\tafn-reserved-future-supply\tafn-future-supply-buyable\nSKU-1\tFNSKU-1\tASIN-1\tWidget\tNew\t12.50\tyes\t1\tyes\t2\t3\t4\t5\t14\t0.25\t6\t7\t8\t0\t1\tyes\n"),
		FBAAllInventoryReportType:              []byte("sku\tfnsku\tasin\tproduct-name\tcondition\tyour-price\tmfn-listing-exists\tmfn-fulfillable-quantity\tafn-listing-exists\tafn-warehouse-quantity\tafn-fulfillable-quantity\tafn-unsellable-quantity\tafn-reserved-quantity\tafn-total-quantity\tper-unit-volume\tafn-inbound-working-quantity\tafn-inbound-shipped-quantity\tafn-inbound-receiving-quantity\tafn-researching-quantity\tafn-reserved-future-supply\tafn-future-supply-buyable\nSKU-1\tFNSKU-1\tASIN-1\tArchived Widget\tNew\t12.50\tyes\t1\tyes\t2\t3\t4\t5\t14\t0.25\t6\t7\t8\t0\t1\tyes\n"),
		ReservedInventoryReportType:            []byte("sku\tfnsku\tasin\tproduct-name\treserved_qty\treserved_customerorders\treserved_fc-transfers\treserved_fc-processing\t\nSKU-1\tFNSKU-1\tASIN-1\tWidget\t8\t2\t3\t3\t\n"),
		AFNInventoryReportType:                 []byte("seller-sku\tfulfillment-channel-sku\tasin\tcondition-type\tWarehouse-Condition-code\tQuantity Available\nSKU-1\tFC-SKU-1\tASIN-1\tNew\tSELLABLE\t17\n"),
		AFNInventoryByCountryReportType:        []byte("seller-sku\tfulfillment-channel-sku\tasin\tcondition-type\tcountry\tquantity-for-local-fulfillment\nSKU-1\tFC-SKU-1\tASIN-1\tNew\tDE\t17\n"),
		FBAStorageFeeChargesReportType:         []byte(strings.Join(fbaStorageFeeChargesHeader, "\t") + "\nx\t" + strings.Repeat("x\t", len(fbaStorageFeeChargesHeader)-2) + "x\n"),
		FBAOverageFeeChargesReportType:         []byte(strings.Join(fbaOverageFeeChargesHeader, "\t") + "\nx\t" + strings.Repeat("x\t", len(fbaOverageFeeChargesHeader)-2) + "x\n"),
		FBALongtermStorageFeeChargesReportType: []byte(strings.Join(fbaLongtermStorageFeeChargesHeader, "\t") + "\nx\t" + strings.Repeat("x\t", len(fbaLongtermStorageFeeChargesHeader)-2) + "x\n"),
		CustomerShipmentReplacementsReportType: []byte("shipment-date\tsku\tasin\tfulfillment-center-id\toriginal-fulfillment-center-id\tquantity\treplacement-reason-code\treplacement-amazon-order-id\toriginal-amazon-order-id\n2026-08-11\tSKU-1\tASIN-1\tFC-1\tFC-2\t2\tDAMAGED\tORDER-2\tORDER-1\n"),
		FBAReimbursementsReportType:            []byte("approval-date\treimbursement-id\tcase-id\tamazon-order-id\treason\tsku\tfnsku\tasin\tproduct-name\tcondition\tcurrency-unit\tamount-per-unit\tamount-total\tquantity-reimbursed-cash\tquantity-reimbursed-inventory\tquantity-reimbursed-total\toriginal-reimbursement-id\toriginal-reimbursement-type\n2026-08-11\tR-1\tC-1\tO-1\tDAMAGED\tSKU-1\tFNSKU-1\tASIN-1\tWidget\tNew\tUSD\t2.00\t4.00\t1\t1\t2\tR-0\tINVENTORY\n"),
		FBAStrandedInventoryReportType:         []byte(strings.Join(fbaStrandedInventoryHeader, "\t") + "\n" + strings.Repeat("x\t", len(fbaStrandedInventoryHeader)-1) + "x\n"),
		FBAEstimatedFeesReportType:             []byte(strings.Join(fbaEstimatedFeesHeader, "\t") + "\n" + strings.Repeat("x\t", len(fbaEstimatedFeesHeader)-1) + "x\n"),
		FBAInboundNoncomplianceReportType:      []byte(strings.Join(fbaInboundNoncomplianceHeader, "\t") + "\n" + strings.Repeat("x\t", len(fbaInboundNoncomplianceHeader)-1) + "x\n"),
		FBARecommendedRemovalReportType:        []byte(strings.Join(fbaRecommendedRemovalHeader, "\t") + "\n" + strings.Repeat("x\t", len(fbaRecommendedRemovalHeader)-1) + "x\n"),
		FBARemovalOrderReportType:              []byte(strings.Join(fbaRemovalOrderHeader, "\t") + "\n" + strings.Repeat("x\t", len(fbaRemovalOrderHeader)-1) + "x\n"),
		FBARemovalShipmentReportType:           []byte(strings.Join(fbaRemovalShipmentHeader, "\t") + "\n" + strings.Repeat("x\t", len(fbaRemovalShipmentHeader)-1) + "x\n"),
		FulfilledShipmentsReportType:           []byte(officialFulfilledShipmentsHeader + "\n" + strings.Repeat("x\t", 47) + "x\n"),
	}
	for reportType, body := range fixtures {
		t.Run(reportType, func(t *testing.T) {
			store := &fakeStore{}
			runner := Runner{Store: store}
			rows, err := runner.saveDownloadedReport(context.Background(), 1, Request{ReportType: reportType}, body, "", "application/octet-stream", "sha", "doc")
			if err != nil || rows != 1 {
				t.Fatalf("save rows=%d err=%v", rows, err)
			}
			switch reportType {
			case FBAInventoryReportType:
				if store.savedFBA != 1 || store.savedFBAAll != 0 || store.savedReserved != 0 || store.savedAFN != 0 {
					t.Fatalf("typed saves=%d/%d/%d/%d", store.savedFBA, store.savedFBAAll, store.savedReserved, store.savedAFN)
				}
			case FBAAllInventoryReportType:
				if store.savedFBAAll != 1 || store.savedFBA != 0 || store.savedReserved != 0 || store.savedAFN != 0 {
					t.Fatalf("typed saves=%d/%d/%d/%d", store.savedFBA, store.savedFBAAll, store.savedReserved, store.savedAFN)
				}
			case ReservedInventoryReportType:
				if store.savedReserved != 1 || store.savedFBA != 0 || store.savedAFN != 0 {
					t.Fatalf("typed saves=%d/%d/%d", store.savedFBA, store.savedReserved, store.savedAFN)
				}
			case AFNInventoryReportType:
				if store.savedAFN != 1 || store.savedFBA != 0 || store.savedReserved != 0 {
					t.Fatalf("typed saves=%d/%d/%d", store.savedFBA, store.savedReserved, store.savedAFN)
				}
			case AFNInventoryByCountryReportType:
				if store.savedAFNByCountry != 1 {
					t.Fatalf("AFN by country saves=%d", store.savedAFNByCountry)
				}
			case FBAStorageFeeChargesReportType:
				if store.savedStorageFees != 1 {
					t.Fatalf("storage fee saves=%d", store.savedStorageFees)
				}
			case FBAOverageFeeChargesReportType:
				if store.savedOverageFees != 1 {
					t.Fatalf("overage fee saves=%d", store.savedOverageFees)
				}
			case FBALongtermStorageFeeChargesReportType:
				if store.savedLongtermFees != 1 {
					t.Fatalf("longterm fee saves=%d", store.savedLongtermFees)
				}
			case CustomerShipmentReplacementsReportType:
				if store.savedReplacements != 1 {
					t.Fatalf("replacement saves=%d", store.savedReplacements)
				}
			case FBAReimbursementsReportType:
				if store.savedReimbursements != 1 {
					t.Fatalf("reimbursement saves=%d", store.savedReimbursements)
				}
			case FBAStrandedInventoryReportType:
				if store.savedStranded != 1 {
					t.Fatalf("stranded saves=%d", store.savedStranded)
				}
			case FBAEstimatedFeesReportType:
				if store.savedEstimatedFees != 1 {
					t.Fatalf("estimated fee saves=%d", store.savedEstimatedFees)
				}
			case FBAInboundNoncomplianceReportType:
				if store.savedNoncompliance != 1 {
					t.Fatalf("noncompliance saves=%d", store.savedNoncompliance)
				}
			case FBARecommendedRemovalReportType:
				if store.savedRecommended != 1 {
					t.Fatalf("recommended saves=%d", store.savedRecommended)
				}
			case FBARemovalOrderReportType:
				if store.savedRemovalOrders != 1 {
					t.Fatalf("removal order saves=%d", store.savedRemovalOrders)
				}
			case FBARemovalShipmentReportType:
				if store.savedRemovalShipments != 1 {
					t.Fatalf("removal shipment saves=%d", store.savedRemovalShipments)
				}
			case FulfilledShipmentsReportType:
				if store.savedFulfilled != 1 {
					t.Fatalf("fulfilled shipment saves=%d", store.savedFulfilled)
				}
			}
		})
	}
}

func TestRunnerCompletesAndRenewsExpiredURLAndAllowsSameRangeRerun(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte("return-date\torder-id\tsku\tasin\tfnsku\tproduct-name\tquantity\tfulfillment-center-id\tdetailed-disposition\treason\tstatus\tlicense-plate-number\tcustomer-comments\n2026-08-11\torder-1\tsku-1\tasin-1\tfnsku-1\tWidget\t2\tFC1\tSELLABLE\tOTHER\tCOMPLETE\tlp-1\tok\n"))
	_ = zw.Close()
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/expired" {
			http.Error(w, "expired", http.StatusForbidden)
			return
		}
		_, _ = w.Write(gz.Bytes())
	}))
	defer download.Close()

	queries := 0
	creates := 0
	renews := 0
	client := signedClientFunc(func(_ context.Context, _ string, path string, body map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			creates++
			if body["report_type"] != CustomerReturnsReportType || body["data_start_time"] != "2026-08-11T00:00:00Z" {
				t.Fatalf("create body = %#v", body)
			}
			return []byte(fmt.Sprintf(`{"code":0,"message":"success","data":{"task_id":"task-%d"}}`, creates)), 200, 0, nil
		case queryPath:
			queries++
			if queries == 1 {
				return []byte(`{"code":0,"data":{"progress_status":"IN_PROGRESS"}}`), 200, 0, nil
			}
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"doc-%d","compression_algorithm":"GZIP","url":"%s/expired"}}`, creates, download.URL)), 200, 0, nil
		case renewPath:
			renews++
			return []byte(fmt.Sprintf(`{"code":0,"data":{"url":"%s/valid","report_document_id":"doc-%d"}}`, download.URL, creates)), 200, 0, nil
		default:
			t.Fatalf("unexpected signed path %s", path)
			return nil, 0, 0, nil
		}
	})
	store := &fakeStore{}
	limiter := &countingLimiter{}
	runner := Runner{Client: client, Store: store, Limiter: limiter, PollInterval: time.Millisecond, PollTimeout: time.Second}
	req := Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"}
	first, err := runner.Run(context.Background(), req)
	if err != nil || first.Status != "SUCCESS" {
		t.Fatalf("first run = %#v, err=%v", first, err)
	}
	second, err := runner.Run(context.Background(), req)
	if err != nil || second.Status != "SUCCESS" {
		t.Fatalf("second run = %#v, err=%v", second, err)
	}
	if creates != 2 || renews != 2 || len(store.audits) != 2 || store.saved != 2 || limiter.waits != 7 {
		t.Fatalf("creates=%d renews=%d audits=%d saved=%d", creates, renews, len(store.audits), store.saved)
	}
}

func TestRunnerRetriesUnknownUntilDone(t *testing.T) {
	data := "return-date\torder-id\tsku\tasin\tfnsku\tproduct-name\tquantity\tfulfillment-center-id\tdetailed-disposition\treason\tstatus\tlicense-plate-number\tcustomer-comments\n2026-08-11\torder-1\tsku-1\tasin-1\tfnsku-1\tWidget\t1\tFC1\tSELLABLE\tOTHER\tCOMPLETE\tlp-1\tok\n"
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(data)) }))
	defer download.Close()
	queries := 0
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			return []byte(`{"code":0,"data":{"task_id":"task-unknown-then-done"}}`), 200, 0, nil
		case queryPath:
			queries++
			switch queries {
			case 1, 2:
				return []byte(`{"code":0,"message":"temporarily unavailable","request_id":"trace-first","data":{"progress_status":"UNKNOWN"}}`), 200, 0, nil
			case 3:
				return []byte(`{"code":0,"data":{"progress_status":"IN_PROGRESS"}}`), 200, 0, nil
			case 4, 5:
				return []byte(`{"code":0,"message":"temporarily unavailable","request_id":"trace-second","data":{"progress_status":"UNKNOWN"}}`), 200, 0, nil
			}
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"doc-1","url":"%s/file"}}`, download.URL)), 200, 0, nil
		default:
			return nil, 0, 0, fmt.Errorf("unexpected path %s", path)
		}
	})
	store := &fakeStore{}
	runner := Runner{Client: client, Store: store, PollInterval: time.Millisecond, PollTimeout: time.Second}
	result, err := runner.Run(context.Background(), Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
	if err != nil || result.Status != "SUCCESS" {
		t.Fatalf("result=%#v, err=%v", result, err)
	}
	wantProgress := []string{"UNKNOWN", "UNKNOWN", "IN_PROGRESS", "UNKNOWN", "UNKNOWN", "DONE"}
	if queries != len(wantProgress) || strings.Join(store.progress, ",") != strings.Join(wantProgress, ",") {
		t.Fatalf("queries=%d progress=%v, want %v", queries, store.progress, wantProgress)
	}
}

func TestRunnerAllowsFourConsecutiveUnknownStatusesBeforeDone(t *testing.T) {
	data := "return-date\torder-id\tsku\tasin\tfnsku\tproduct-name\tquantity\tfulfillment-center-id\tdetailed-disposition\treason\tstatus\tlicense-plate-number\tcustomer-comments\n2026-08-11\torder-1\tsku-1\tasin-1\tfnsku-1\tWidget\t1\tFC1\tSELLABLE\tOTHER\tCOMPLETE\tlp-1\tok\n"
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(data)) }))
	defer download.Close()

	queries := 0
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			return []byte(`{"code":0,"data":{"task_id":"task-four-unknown-then-done"}}`), 200, 0, nil
		case queryPath:
			queries++
			if queries <= 4 {
				return []byte(`{"code":0,"message":"success","data":{"progress_status":"UNKNOWN"}}`), 200, 0, nil
			}
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"doc-1","url":"%s/file"}}`, download.URL)), 200, 0, nil
		default:
			return nil, 0, 0, fmt.Errorf("unexpected path %s", path)
		}
	})
	store := &fakeStore{}
	runner := Runner{Client: client, Store: store, PollInterval: time.Millisecond, PollTimeout: time.Second}
	result, err := runner.Run(context.Background(), Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
	if err != nil || result.Status != "SUCCESS" {
		t.Fatalf("result=%#v, err=%v", result, err)
	}
	if queries != 5 {
		t.Fatalf("queries=%d, want four UNKNOWN polls followed by DONE", queries)
	}
}

func TestRunnerReportsUnknownStatusDiagnosticsAfterTimeout(t *testing.T) {
	queries := 0
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			return []byte(`{"code":0,"data":{"task_id":"task-unknown"}}`), 200, 0, nil
		case queryPath:
			queries++
			return []byte(`{"code":0,"message":"upstream detail","request_id":"trace-123","error_details":["seller report unavailable"],"data":{"progress_status":"UNKNOWN"}}`), 200, 0, nil
		default:
			return nil, 0, 0, fmt.Errorf("unexpected path %s", path)
		}
	})
	store := &fakeStore{}
	runner := Runner{Client: client, Store: store, PollInterval: time.Millisecond, PollTimeout: 10 * time.Millisecond}
	_, err := runner.Run(context.Background(), Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
	if err == nil || !strings.Contains(err.Error(), "polling timed out") || !strings.Contains(err.Error(), "last progress_status=UNKNOWN") || !strings.Contains(err.Error(), "request_id=trace-123") || !strings.Contains(err.Error(), "message=\"upstream detail\"") || !strings.Contains(err.Error(), "seller report unavailable") {
		t.Fatalf("unknown status error=%v", err)
	}
	if queries <= 3 {
		t.Fatalf("queries=%d, want UNKNOWN polling to continue past three responses", queries)
	}
}

func TestResponseDiagnosticsRedactsSensitiveMessageAndDetails(t *testing.T) {
	raw := []byte(`{"request_id":"trace-429","message":"access_token=do-not-leak","error_details":{"url":"https://example.test/?token=do-not-leak","reason":"too many requests"}}`)
	got := responseDiagnostics(raw)
	if strings.Contains(got, "do-not-leak") {
		t.Fatalf("diagnostics=%q, must redact credentials", got)
	}
}

func TestDecodeEnvelopeRedactsSensitiveErrorMessage(t *testing.T) {
	var target struct {
		TaskID string `json:"task_id"`
	}
	err := decodeEnvelope([]byte(`{"code":429,"message":"app_secret=do-not-leak","data":null}`), &target)
	if err == nil {
		t.Fatal("decodeEnvelope should return the upstream error")
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("error=%q, must redact credentials", err)
	}
}

func TestRunnerTerminalDiagnosticsAreSanitizedForAllNonDoneStatuses(t *testing.T) {
	for _, status := range []string{"FATAL", "CANCELLED", "UNKNOWN"} {
		t.Run(status, func(t *testing.T) {
			client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
				switch path {
				case createPath:
					return []byte(`{"code":0,"data":{"task_id":"task-diagnostics"}}`), http.StatusOK, 0, nil
				case queryPath:
					return []byte(`{"code":0,"message":"upstream app_secret=do-not-leak","request_id":"trace-terminal","error_details":{"reason":"detail","access_token":"do-not-leak"},"data":{"progress_status":"` + status + `"}}`), http.StatusOK, 0, nil
				default:
					return nil, 0, 0, fmt.Errorf("unexpected signed path %s", path)
				}
			})
			store := &fakeStore{}
			runner := Runner{Client: client, Store: store, PollInterval: time.Millisecond, PollTimeout: 5 * time.Millisecond}
			_, err := runner.Run(context.Background(), Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"market"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
			if err == nil {
				t.Fatal("expected terminal report error")
			}
			if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), `"data"`) {
				t.Fatalf("error=%q, must not expose secret or raw envelope", err)
			}
			if !strings.Contains(err.Error(), "request_id=trace-terminal") || !strings.Contains(err.Error(), "error_details=") {
				t.Fatalf("error=%q, want sanitized diagnostics", err)
			}
			if len(err.Error()) > 1024 {
				t.Fatalf("error length=%d, want bounded diagnostics", len(err.Error()))
			}
		})
	}
}

func TestRunnerDefaultPollingBoundaryIsTwentyFourHours(t *testing.T) {
	if defaultPollInterval != time.Minute {
		t.Fatalf("default polling interval=%s, want one minute", defaultPollInterval)
	}
	if defaultPollTimeout != 24*time.Hour {
		t.Fatalf("default polling boundary=%s, want official 24h task timeout", defaultPollTimeout)
	}
}

func TestRunnerUsesOneMinuteWhenPollIntervalIsUnconfigured(t *testing.T) {
	queries := 0
	runner := Runner{
		Client: signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
			if path != queryPath {
				return nil, 0, 0, fmt.Errorf("unexpected signed path %s", path)
			}
			queries++
			return []byte(`{"code":0,"data":{"progress_status":"IN_PROGRESS"}}`), http.StatusOK, 0, nil
		}),
		Store:       &fakeStore{},
		PollTimeout: time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	_, err := runner.waitForDone(ctx, Request{SellerID: "seller", Region: "na"}, 1, "task-1")
	if err != context.DeadlineExceeded {
		t.Fatalf("waitForDone error=%v, want context deadline", err)
	}
	if queries != 1 {
		t.Fatalf("queries=%d before one minute default interval, want 1", queries)
	}
}

type concurrentStore struct {
	mu      sync.Mutex
	audit   Audit
	ready   chan struct{}
	ensures int
	saved   int
}

func (s *concurrentStore) EnsureReport(context.Context, Request) (Audit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensures++
	if s.audit.ID != 0 {
		s.audit.CreateClaimed = false
		return s.audit, nil
	}
	s.audit = Audit{ID: 1, Status: "CREATING", CreateClaimed: true}
	if s.ready != nil {
		close(s.ready)
		s.ready = nil
	}
	return s.audit, nil
}
func (s *concurrentStore) LoadReport(_ context.Context, _ int64) (Audit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Audit{ID: s.audit.ID, ReportTaskID: s.audit.ReportTaskID, Status: s.audit.Status}, nil
}
func (s *concurrentStore) MarkReportCreated(_ context.Context, _ int64, taskID string) error {
	s.mu.Lock()
	s.audit.ReportTaskID = taskID
	s.mu.Unlock()
	return nil
}
func (s *concurrentStore) MarkReportProgress(context.Context, int64, string, string, string, string) error {
	return nil
}
func (s *concurrentStore) SaveCustomerReturns(_ context.Context, _ int64, rows []CustomerReturn, _ string, _ string) error {
	s.mu.Lock()
	s.saved += len(rows)
	s.audit.Status = "SUCCESS"
	s.mu.Unlock()
	return nil
}
func (s *concurrentStore) MarkReportError(context.Context, int64, string, error) error { return nil }

func TestRunnerConcurrentCallsCreateOnceForActiveScope(t *testing.T) {
	data := "return-date\torder-id\tsku\tasin\tfnsku\tproduct-name\tquantity\tfulfillment-center-id\tdetailed-disposition\treason\tstatus\tlicense-plate-number\tcustomer-comments\n2026-08-11\to-1\ts-1\ta-1\tf-1\tWidget\t1\tFC1\tSELLABLE\tOTHER\tCOMPLETE\tlp-1\tok\n"
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(data)) }))
	defer download.Close()
	ready := make(chan struct{})
	store := &concurrentStore{ready: ready}
	var mu sync.Mutex
	creates := 0
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			<-ready
			mu.Lock()
			creates++
			mu.Unlock()
			return []byte(`{"code":0,"data":{"task_id":"shared-task"}}`), 200, 0, nil
		case queryPath:
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"shared-doc","url":"%s/file"}}`, download.URL)), 200, 0, nil
		default:
			return nil, 0, 0, fmt.Errorf("unexpected signed path %s", path)
		}
	})
	request := Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"market"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"}
	runner := Runner{Client: client, Store: store, PollInterval: time.Millisecond, PollTimeout: time.Second}
	results := make(chan error, 2)
	go func() { _, err := runner.Run(context.Background(), request); results <- err }()
	go func() { _, err := runner.Run(context.Background(), request); results <- err }()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent runner error: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 {
		t.Fatalf("upstream create calls = %d, want 1", creates)
	}
}

func TestRunnerSharedTaskWaitDefaultsToTenSeconds(t *testing.T) {
	store := &concurrentStore{audit: Audit{ID: 1, Status: "CREATING", CreateClaimed: false}}
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	runner := Runner{
		Client: signedClientFunc(func(context.Context, string, string, map[string]any) ([]byte, int, int, error) {
			t.Fatal("shared waiter must not call upstream before task id exists")
			return nil, 0, 0, nil
		}),
		Store:        store,
		PollInterval: time.Millisecond,
		PollTimeout:  time.Hour,
	}
	started := time.Now()
	_, err := runner.Run(ctx, Request{
		AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"market"},
		DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-12T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "10s") {
		t.Fatalf("error = %v, want fixed 10s shared-task timeout", err)
	}
	if elapsed := time.Since(started); elapsed < 9*time.Second || elapsed > 12*time.Second {
		t.Fatalf("shared wait elapsed = %s, want about 10s", elapsed)
	}
}

func TestRunnerDoesNotPersistRowsWhenTSVParsingFails(t *testing.T) {
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("return-date\torder-id\n"))
	}))
	defer download.Close()
	client := signedClientFunc(func(_ context.Context, _ string, path string, _ map[string]any) ([]byte, int, int, error) {
		switch path {
		case createPath:
			return []byte(`{"code":0,"data":{"task_id":"task-1"}}`), 200, 0, nil
		case queryPath:
			return []byte(fmt.Sprintf(`{"code":0,"data":{"progress_status":"DONE","report_document_id":"doc-1","compression_algorithm":"","url":"%s/file"}}`, download.URL)), 200, 0, nil
		default:
			return nil, 0, 0, fmt.Errorf("unexpected path %s", path)
		}
	})
	store := &fakeStore{}
	runner := Runner{Client: client, Store: store, PollTimeout: time.Second}
	_, err := runner.Run(context.Background(), Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"})
	if err == nil {
		t.Fatal("expected TSV parsing failure")
	}
	if store.saved != 0 || store.errors != 1 {
		t.Fatalf("saved=%d errors=%d", store.saved, store.errors)
	}
}

func TestRunnerReportsAuditWriteFailureAlongsideOriginalError(t *testing.T) {
	client := signedClientFunc(func(context.Context, string, string, map[string]any) ([]byte, int, int, error) {
		return nil, 500, 0, fmt.Errorf("create failed")
	})
	store := &fakeStore{markErrorErr: fmt.Errorf("audit write failed")}
	runner := Runner{Client: client, Store: store}
	_, err := runner.Run(context.Background(), Request{
		AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"},
		DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z",
	})
	if err == nil || !strings.Contains(err.Error(), "create failed") || !strings.Contains(err.Error(), "audit write failed") {
		t.Fatalf("error = %v, want original and audit write failures", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestDownloadAppliesDefaultTimeout(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok {
			t.Fatal("download request has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > defaultDownloadTimeout {
			t.Fatalf("download deadline remaining = %s", remaining)
		}
		return nil, fmt.Errorf("stop after deadline assertion")
	})}
	runner := Runner{HTTP: httpClient}
	_, _, _, err := runner.download(context.Background(), Request{}, reportStatus{URL: "https://example.invalid/report"})
	if err == nil {
		t.Fatal("expected test transport error")
	}
}

func TestDownloadPreservesReportContentType(t *testing.T) {
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=cp1252")
		_, _ = w.Write([]byte("report"))
	}))
	defer download.Close()

	runner := Runner{}
	_, _, contentType, err := runner.download(context.Background(), Request{}, reportStatus{URL: download.URL})
	if err != nil {
		t.Fatalf("download returned error: %v", err)
	}
	if contentType != "text/plain; charset=cp1252" {
		t.Fatalf("content type = %q, want cp1252 declaration", contentType)
	}
}

func TestReportCallDoesNotRetryHTTP200BusinessCode429(t *testing.T) {
	calls := 0
	client := signedClientFunc(func(_ context.Context, _, _ string, _ map[string]any) ([]byte, int, int, error) {
		calls++
		return nil, http.StatusOK, http.StatusTooManyRequests, api.NewFetchError(http.StatusOK, http.StatusTooManyRequests, "请求过于频繁", time.Millisecond, true)
	})
	runner := Runner{Client: client}
	_, err := runner.call(context.Background(), queryPath, map[string]any{"task_id": "task-1"})
	if err == nil {
		t.Fatal("call should return the HTTP 200 business error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want one request for HTTP 200 business code 429", calls)
	}
}

func TestReportCallRetriesHTTP429(t *testing.T) {
	calls := 0
	client := signedClientFunc(func(_ context.Context, _, _ string, _ map[string]any) ([]byte, int, int, error) {
		calls++
		if calls == 1 {
			return nil, http.StatusTooManyRequests, 0, api.NewFetchError(http.StatusTooManyRequests, 0, "rate limited", time.Millisecond, true)
		}
		return []byte(`{"code":0,"data":{"progress_status":"IN_PROGRESS"}}`), http.StatusOK, 0, nil
	})
	runner := Runner{Client: client}
	raw, err := runner.call(context.Background(), queryPath, map[string]any{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("call after HTTP 429 = %v", err)
	}
	if calls != 2 || !strings.Contains(string(raw), "IN_PROGRESS") {
		t.Fatalf("calls=%d raw=%s", calls, raw)
	}
}

func TestReportCallRetriesOfficialBusinessCode3001008(t *testing.T) {
	calls := 0
	client := signedClientFunc(func(_ context.Context, _, _ string, _ map[string]any) ([]byte, int, int, error) {
		calls++
		if calls == 1 {
			return nil, http.StatusOK, 3001008, api.NewFetchError(http.StatusOK, 3001008, "new requests too frequently", time.Millisecond, true)
		}
		return []byte(`{"code":0,"data":{"progress_status":"IN_PROGRESS"}}`), http.StatusOK, 0, nil
	})
	runner := Runner{Client: client}
	if _, err := runner.call(context.Background(), queryPath, map[string]any{"task_id": "task-1"}); err != nil {
		t.Fatalf("call after business 3001008 = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want retry for official business code 3001008", calls)
	}
}

func TestValidateRequestRejectsInvalidRegionAndBlankMarketplace(t *testing.T) {
	base := Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"}, DateFrom: "2026-08-01T00:00:00Z", DateTo: "2026-08-02T00:00:00Z"}
	for name, request := range map[string]Request{
		"unsupported report type": func() Request { r := base; r.ReportType = "GET_UNSUPPORTED_REPORT"; return r }(),
		"invalid region":          func() Request { r := base; r.Region = "ap"; return r }(),
		"blank marketplace":       func() Request { r := base; r.MarketplaceIDs = []string{" "}; return r }(),
		"duplicate marketplace":   func() Request { r := base; r.MarketplaceIDs = []string{"ATVPDHSKDCJ6R", "ATVPDHSKDCJ6R"}; return r }(),
		"blank seller":            func() Request { r := base; r.SellerID = " seller "; return r }(),
		"blank account":           func() Request { r := base; r.AccountID = " acct "; return r }(),
		"blank store":             func() Request { r := base; r.StoreID = ""; return r }(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRequest(request); err == nil {
				t.Fatal("expected request validation error")
			}
		})
	}
}

func TestValidateRequestAllowsAllOrdersReportType(t *testing.T) {
	request := Request{
		ReportType:     AllOrdersReportType,
		AccountID:      "acct",
		SellerID:       "seller",
		StoreID:        "store-1",
		Region:         "na",
		MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		DateFrom:       "2026-08-01T00:00:00Z",
		DateTo:         "2026-08-02T23:59:59Z",
	}
	if err := validateRequest(request); err != nil {
		t.Fatalf("all orders request rejected before task creation: %v", err)
	}
}

func TestActiveScopeKeyCanonicalizesMarketplaceOrderAndRFC3339Offset(t *testing.T) {
	left := Request{AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"B", "A"}, DateFrom: "2026-08-01T00:00:00Z", DateTo: "2026-08-02T00:00:00Z"}
	right := left
	right.MarketplaceIDs = []string{"A", "B"}
	right.DateFrom = "2026-07-31T20:00:00-04:00"
	right.DateTo = "2026-08-01T20:00:00-04:00"
	if ActiveScopeKey(left) != ActiveScopeKey(right) {
		t.Fatal("equivalent marketplace/date scope produced different active keys")
	}
}

func TestValidateRequestRejectsOverlongDateRange(t *testing.T) {
	err := validateRequest(Request{
		AccountID: "acct", SellerID: "seller", StoreID: "store-1", Region: "na", MarketplaceIDs: []string{"ATVPDHSKDCJ6R"},
		DateFrom: "2026-01-01T00:00:00Z", DateTo: "2026-02-02T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected date range limit error")
	}
}
