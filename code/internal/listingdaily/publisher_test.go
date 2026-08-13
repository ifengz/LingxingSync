package listingdaily

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAssembleReportValuesOverrideAPIWithoutFillingUnknown(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	apiUnits, apiAmount, reportUnits := int64(3), 12.5, int64(5)
	api := []Input{{
		Key:    Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date},
		Scope:  ScopeListing,
		Values: Values{SalesUnits: &apiUnits, SalesAmount: &apiAmount},
	}}
	report := []Input{{
		Key:    api[0].Key,
		Scope:  ScopeListing,
		Values: Values{SalesUnits: &reportUnits},
	}}

	got, err := Assemble(api, report, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Values.SalesUnits == nil || *got[0].Values.SalesUnits != reportUnits {
		t.Fatalf("report value was not preferred: %#v", got)
	}
	if got[0].Values.SalesAmount == nil || *got[0].Values.SalesAmount != apiAmount {
		t.Fatalf("API fallback was not preserved: %#v", got)
	}
	if got[0].Values.ReturnsQty != nil {
		t.Fatalf("unknown source was filled: %#v", got[0].Values.ReturnsQty)
	}
	if got[0].Sources["sales_units"] != SourceReport || got[0].Sources["sales_amount"] != SourceAPI {
		t.Fatalf("field sources = %#v", got[0].Sources)
	}
}

func TestAssembleAllowsOnlyExplicitStoreScopedHSA(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	storeScoped := Input{Key: Key{Store: "store-1", Channel: "hsa", BusinessDate: date}, Scope: ScopeStore, Values: Values{HSASpend: ptrFloat(1)}}
	if _, err := Assemble([]Input{storeScoped}, nil, false); err != nil {
		t.Fatalf("store-scoped HSA rejected: %v", err)
	}
	listingWithoutSKU := storeScoped
	listingWithoutSKU.Scope = ScopeListing
	if _, err := Assemble([]Input{listingWithoutSKU}, nil, false); err == nil {
		t.Fatal("listing input without ASIN/SKU was accepted")
	}
}

func TestBusinessDatePreservesSourceCalendarDateAcrossOffsets(t *testing.T) {
	for _, inputDate := range []time.Time{
		time.Date(2026, 8, 12, 0, 30, 0, 0, time.FixedZone("+08", 8*60*60)),
		time.Date(2026, 8, 12, 0, 30, 0, 0, time.FixedZone("+10", 10*60*60)),
	} {
		rows, err := Assemble([]Input{{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: inputDate}, Scope: ScopeListing}}, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if got := rows[0].Key.BusinessDate.Format("2006-01-02"); got != "2026-08-12" {
			t.Fatalf("business date = %s for %s", got, inputDate)
		}
	}
}

func TestReconcileReportsMissingAndFieldDiffs(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	leftUnits, rightUnits := int64(1), int64(2)
	common := Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}
	got, err := Reconcile(
		[]Metric{{Key: common, Scope: ScopeListing, Values: Values{SalesUnits: &leftUnits}}, {Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B03", SKU: "SKU-3", BusinessDate: date}, Scope: ScopeListing}},
		[]Metric{{Key: common, Scope: ScopeListing, Values: Values{SalesUnits: &rightUnits}}, {Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B02", SKU: "SKU-2", BusinessDate: date}, Scope: ScopeListing}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.MissingInDB) != 1 || len(got.MissingInReport) != 1 || len(got.FieldDiffs) != 1 {
		t.Fatalf("unexpected reconciliation: %#v", got)
	}
}

func TestPublishKeepsCurrentDayProvisionalAndUnknownNull(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	batch := Batch{ReportState: ReportAbsent, Rows: []Metric{{
		Key:   Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date},
		Scope: ScopeListing,
	}}}
	if err := Publish(context.Background(), store, batch, date); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 || !store.rows[0].IsProvisional || store.rows[0].IsVerified {
		t.Fatalf("publication state = %#v", store.rows)
	}
	if store.rows[0].Values.InventorySellable != nil || store.rows[0].Values.InventoryInbound != nil || store.rows[0].Values.SessionsTotal != nil || store.rows[0].Values.SDSpend != nil {
		t.Fatalf("unknown values must remain NULL: %#v", store.rows[0].Values)
	}
}

func TestReturnsReportOnlyVerifiesReturnsField(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	returns := int64(2)
	rows, err := Assemble(
		[]Input{{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: ptrInt(3)}}},
		[]Input{{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &returns}}},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{}
	if err := Publish(context.Background(), store, Batch{Rows: rows, ReportState: ReportReconciled}, date.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if !store.rows[0].VerifiedFields["returns_qty"] || store.rows[0].IsVerified {
		t.Fatalf("returns-only verification leaked to full row: %#v", store.rows[0])
	}
}

func TestProjectAndPublishUsesRawBoundaryAndRefusesFailedReport(t *testing.T) {
	store := &fakeStore{}
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	record := RawRecord{Source: SourceAPI, Input: Input{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{SessionsTotal: ptrInt(9)}}}
	if err := ProjectAndPublish(context.Background(), store, []RawRecord{record}, nil, ReportAbsent, date); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 || store.rows[0].Values.SessionsTotal == nil || *store.rows[0].Values.SessionsTotal != 9 {
		t.Fatalf("raw projection did not publish: %#v", store.rows)
	}
	if err := ProjectAndPublish(context.Background(), store, []RawRecord{record}, nil, ReportFailed, date); err == nil {
		t.Fatal("failed report published a partial batch")
	}
}

func TestMetricArgsContainsEveryAuthorizedFieldAndProvenance(t *testing.T) {
	values := Values{
		SalesUnits: ptrInt(1), SalesAmount: ptrFloat(2), ReturnsQty: ptrInt(3), InventorySellable: ptrInt(4), InventoryInbound: ptrInt(5), InventoryReserved: ptrInt(6), InventoryUnfulfillable: ptrInt(7), InventoryLocalWarehouse: ptrInt(8), InventoryUnhealthyUnits: ptrInt(9), InventoryAged90SellableUnits: ptrInt(10), InventorySellThroughRate: ptrFloat(11), InventoryReceiveFillRate: ptrFloat(12), InventoryVendorConfirmationRate: ptrFloat(13), InventoryAvgLeadTimeDays: ptrFloat(14), InventorySellableCost: ptrFloat(15), InventoryUnfulfillableCost: ptrFloat(16), InventoryAged90Cost: ptrFloat(17), InventoryUnhealthyCost: ptrFloat(18), InventoryInboundCost: ptrFloat(19), InventoryCurrency: ptrString("USD"),
		InventoryInboundReceiving: ptrInt(20), InventoryInboundShipped: ptrInt(21), InventoryInboundWorking: ptrInt(22), InventoryReservedCustomerOrders: ptrInt(23), InventoryReservedFCProcessing: ptrInt(24), InventoryReservedFCTransfers: ptrInt(25),
		SessionsDesktop: ptrInt(26), SessionsMobile: ptrInt(27), SessionsTotal: ptrInt(28), ReviewCount: ptrInt(29), Rating: ptrFloat(4.2),
		SPSpend: ptrFloat(30), SPSales: ptrFloat(31), SPOrders: ptrInt(32), SDSpend: ptrFloat(33), SDSales: ptrFloat(34), SDOrders: ptrInt(35),
		HSASpend: ptrFloat(36), HSASales: ptrFloat(37), HSAOrders: ptrInt(38), SBSpend: ptrFloat(39), SBSales: ptrFloat(40), SBOrders: ptrInt(41),
	}
	row := Metric{Values: values, Sources: map[string]Source{}}
	for _, field := range knownFields(values) {
		row.Sources[field] = SourceAPI
	}
	if got := metricArgs(1, row); len(got) != 92 {
		t.Fatalf("metric args = %d, want 92", len(got))
	}
}

func TestMetricsUpsertSQLPreventsAPIAfterReportRegression(t *testing.T) {
	want := "(VALUES(sales_units_source) = 'report' OR (VALUES(sales_units_source) = 'api' AND sales_units_source <> 'report'))"
	if !strings.Contains(metricsUpsertSQL, want) {
		t.Fatalf("sales source precedence is not explicit: %s", metricsUpsertSQL)
	}
	if strings.Contains(metricsUpsertSQL, "VALUES(sales_units_source) <> '' AND") {
		t.Fatal("legacy ambiguous source precedence remains")
	}
}

func TestPublishBatchDoesNotWriteWhenReportReconciliationFails(t *testing.T) {
	store := &fakeStore{}
	batch := Batch{ReportState: ReportFailed, Rows: []Metric{{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: time.Now().UTC()}, Scope: ScopeListing}}}
	if err := Publish(context.Background(), store, batch, time.Now()); err == nil {
		t.Fatal("failed report reconciliation was silently published")
	}
	if len(store.rows) != 0 {
		t.Fatalf("failed batch wrote %d rows", len(store.rows))
	}
}

func TestPrepareBuildsRowsWithoutWriting(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	units := int64(2)
	rows, err := Prepare(Batch{ReportState: ReportAbsent, Rows: []Metric{{
		Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: &units},
	}}}, date)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].IsProvisional || rows[0].Sources["sales_units"] != SourceAPI {
		t.Fatalf("prepared rows = %#v", rows)
	}
}

type fakeStore struct{ rows []Metric }

func (f *fakeStore) Persist(_ context.Context, rows []Metric) error {
	f.rows = append(f.rows, rows...)
	return nil
}

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int64) *int64       { return &v }
func ptrString(v string) *string  { return &v }
