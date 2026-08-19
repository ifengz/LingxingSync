package worker

import (
	"reflect"
	"strings"
	"testing"

	"lingxing-sync/internal/db"
)

func TestSalesOrderDetailBatchesKeepStoreAnd200OrderLimit(t *testing.T) {
	candidates := make([]db.SalesOrderCandidate, 0, 202)
	for i := 0; i < 201; i++ {
		candidates = append(candidates, db.SalesOrderCandidate{SID: "store-a", AmazonOrderID: "order-" + string(rune('a'+i%26)) + "-" + strings.Repeat("x", i/26)})
	}
	candidates = append(candidates, db.SalesOrderCandidate{SID: "store-b", AmazonOrderID: "order-b"})
	batches := salesOrderDetailBatches(candidates)
	if len(batches) != 3 || len(batches[0]) != 200 || len(batches[1]) != 1 || len(batches[2]) != 1 {
		t.Fatalf("batch sizes = %d/%d/%d, want 200/1/1", len(batches[0]), len(batches[1]), len(batches[2]))
	}
	if batches[2][0].SID != "store-b" {
		t.Fatalf("second store must not share its batch: %#v", batches[2])
	}
}

func TestDetailCandidateParamsSendsOnlyCommaSeparatedOrderIDs(t *testing.T) {
	params, sids, err := detailCandidateParams([]db.SalesOrderCandidate{{SID: "store-a", AmazonOrderID: "order-1"}, {SID: "store-a", AmazonOrderID: "order-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(params, map[string]any{"order_id": "order-1,order-2"}) || !reflect.DeepEqual(sids, map[string]string{"order-1": "store-a", "order-2": "store-a"}) {
		t.Fatalf("detail request identity = params=%#v sids=%#v", params, sids)
	}
}

func TestShapeSalesOrderDetailRowsRequiresExactReturnedIdentity(t *testing.T) {
	expected := map[string]string{"order-1": "store-a", "order-2": "store-a"}
	rows := []map[string]any{{"amazon_order_id": "order-1", "sid": "store-a"}, {"amazon_order_id": "order-2", "sid": "store-a"}}
	shaped, err := shapeSalesOrderDetailRows(rows, expected)
	if err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	if !reflect.DeepEqual(shaped, rows) {
		t.Fatalf("shaped rows = %#v, want %#v", shaped, rows)
	}
	for name, rows := range map[string][]map[string]any{
		"unknown order": {{"amazon_order_id": "order-x", "sid": "store-a"}},
		"wrong store":   {{"amazon_order_id": "order-1", "sid": "store-b"}, {"amazon_order_id": "order-2", "sid": "store-a"}},
		"missing order": {{"amazon_order_id": "order-1", "sid": "store-a"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := shapeSalesOrderDetailRows(rows, expected); err == nil {
				t.Fatal("invalid detail response was accepted")
			}
		})
	}
}

func TestShapeSalesOrderDetailRowsDeduplicatesOnlyEquivalentRows(t *testing.T) {
	expected := map[string]string{"order-1": "store-a", "order-2": "store-a"}
	first := map[string]any{
		"amazon_order_id": "order-1",
		"sid":             "store-a",
		"item_list":       []any{map[string]any{"order_item_id": "item-1", "quantity_shipped": float64(1)}},
	}
	equivalent := map[string]any{
		"item_list":       []any{map[string]any{"quantity_shipped": float64(1), "order_item_id": "item-1"}},
		"sid":             "store-a",
		"amazon_order_id": "order-1",
	}
	second := map[string]any{"amazon_order_id": "order-2", "sid": "store-a"}
	shaped, err := shapeSalesOrderDetailRows([]map[string]any{first, equivalent, second}, expected)
	if err != nil {
		t.Fatalf("equivalent duplicate rejected: %v", err)
	}
	if !reflect.DeepEqual(shaped, []map[string]any{first, second}) {
		t.Fatalf("shaped rows = %#v", shaped)
	}

	conflicting := map[string]any{
		"amazon_order_id": "order-1",
		"sid":             "store-a",
		"item_list":       []any{map[string]any{"order_item_id": "item-1", "quantity_shipped": float64(2)}},
	}
	if _, err := shapeSalesOrderDetailRows([]map[string]any{first, conflicting, second}, expected); err == nil || !strings.Contains(err.Error(), "非等价重复") {
		t.Fatalf("conflicting duplicate error = %v, want non-equivalent duplicate", err)
	}
}
