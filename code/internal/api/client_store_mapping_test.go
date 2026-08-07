package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
)

func TestFetchMapsSCStoreFields(t *testing.T) {
	client := newStoreMappingTestClient(t, "/erp/sc/data/seller/lists", `{
		"code": 0,
		"data": {"list": [{
			"sid": "10558",
			"name": "SC Brazil Store",
			"region": "BR",
			"has_ads_setting": 1,
			"status": 1
		}], "total": 1, "has_more": false}
	}`)

	result, _, _, err := client.Fetch(context.Background(), http.MethodGet, "/erp/sc/data/seller/lists", nil)
	if err != nil {
		t.Fatalf("Fetch SC stores: %v", err)
	}
	got := result.List[0]
	if got["sid"] != "10558" || got["store_name"] != "SC Brazil Store" || got["country"] != "BR" || got["store_type"] != "SC" {
		t.Fatalf("SC store mapping = %#v", got)
	}
}

func TestFetchMapsVCStoreFields(t *testing.T) {
	client := newStoreMappingTestClient(t, "/basicOpen/platformAuth/vcSeller/pageList", `{
		"code": 0,
		"data": {"list": [{
			"account_id": "vendor-account",
			"seller_id": "vendor-seller",
			"vc_store_id": "13461850906074624",
			"name": "clicktech-India vc",
			"region": "IN",
			"status": 1,
			"mid": "marketplace"
		}], "total": 1, "has_more": false}
	}`)

	result, _, _, err := client.Fetch(context.Background(), http.MethodPost, "/basicOpen/platformAuth/vcSeller/pageList", map[string]any{"offset": 0, "length": 200})
	if err != nil {
		t.Fatalf("Fetch VC stores: %v", err)
	}
	got := result.List[0]
	if got["sid"] != "13461850906074624" || got["store_name"] != "clicktech-India vc" || got["country"] != "IN" || got["store_type"] != "VC" {
		t.Fatalf("VC store mapping = %#v", got)
	}
}

func TestFetchRejectsVCStoreWithoutStoreID(t *testing.T) {
	client := newStoreMappingTestClient(t, "/basicOpen/platformAuth/vcSeller/pageList", `{
		"code": 0,
		"data": {"list": [{"name": "VC Store", "region": "IN"}], "total": 1, "has_more": false}
	}`)

	_, _, _, err := client.Fetch(context.Background(), http.MethodPost, "/basicOpen/platformAuth/vcSeller/pageList", map[string]any{"offset": 0, "length": 200})
	if err == nil || !strings.Contains(err.Error(), "vc_store_id") {
		t.Fatalf("missing vc_store_id error = %v", err)
	}
}

func newStoreMappingTestClient(t *testing.T, businessPath, response string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case tokenEndpoint:
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
		case businessPath:
			_, _ = w.Write([]byte(response))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
}
