package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

var errDetailTest = errors.New("detail failed")

func TestFetchVCPODetailSendsOnlyLocalPONumberAndForceInjectsStoreContext(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth-server/oauth/access-token" {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		if r.URL.Path != "/basicOpen/platformOrder/vcOrderPo/detail" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode detail body: %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"purchase_order_number":"678OWC8S","local_po_number":"response-value","vc_store_id":"response-store","purchase_order_state":"Acknowledged","items":[{"asin":"B01","msku":"SKU-1"}]}}`))
	}))
	t.Cleanup(server.Close)

	account := &config.Account{ID: "sc_us_1", AppKey: "1234567890abcdef", AppSecret: "secret"}
	w := &EndpointWorker{
		Endpoint: config.Endpoint{
			Name: "vc_po_details_sc_us_1", Method: http.MethodPost,
			Path: "/basicOpen/platformOrder/vcOrderPo/detail", ResponseShape: "object",
			ForceInjectParams: []string{"vc_store_id", "local_po_number"},
		},
		Account:  *account,
		Client:   api.NewClient(account, server.URL),
		Limiters: NewLimiterRegistry(),
	}

	row, _, _, _, err := w.fetchVCPODetail(context.Background(), NewLimiter(1, 1), db.VCPOCandidate{
		VCStoreID: "134710151768940032", LocalPONumber: "402731513323165538",
	})
	if err != nil {
		t.Fatalf("fetch VC PO detail: %v", err)
	}
	if !reflect.DeepEqual(gotBody, map[string]any{"local_po_number": "402731513323165538"}) {
		t.Fatalf("detail request body = %#v, want only local_po_number", gotBody)
	}
	if row["vc_store_id"] != "134710151768940032" || row["local_po_number"] != "402731513323165538" {
		t.Fatalf("candidate identity was not force injected: %#v", row)
	}
	if _, ok := row["items"].([]any); !ok {
		t.Fatalf("items must remain nested JSON value, got %T", row["items"])
	}
}

func TestForEachVCPODetailCandidateStopsOnFirstError(t *testing.T) {
	candidates := []db.VCPOCandidate{
		{VCStoreID: "store-1", LocalPONumber: "po-1"},
		{VCStoreID: "store-2", LocalPONumber: "po-2"},
		{VCStoreID: "store-3", LocalPONumber: "po-3"},
	}
	visited := make([]string, 0, len(candidates))
	completed, err := forEachVCPODetailCandidate(candidates, func(_ int, candidate db.VCPOCandidate) error {
		visited = append(visited, candidate.LocalPONumber)
		if candidate.LocalPONumber == "po-2" {
			return errDetailTest
		}
		return nil
	})
	if err != errDetailTest {
		t.Fatalf("first detail error = %v, want %v", err, errDetailTest)
	}
	if completed != 1 || !reflect.DeepEqual(visited, []string{"po-1", "po-2"}) {
		t.Fatalf("completed=%d visited=%v, want one completed and stop at second", completed, visited)
	}
}

func TestVCOrdersRecordIDsRequireStoreScope(t *testing.T) {
	if vcOrdersRecordIDsValid([]string{"local_po_number"}) {
		t.Fatal("VC PO 列表不能继续使用缺少店铺的旧 record_id_fields")
	}
	if !vcOrdersRecordIDsValid([]string{"vc_store_id", "local_po_number"}) {
		t.Fatal("VC PO 列表必须接受店铺+PO 号业务键")
	}
}
