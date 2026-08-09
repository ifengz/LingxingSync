package worker

import (
	"reflect"
	"testing"

	"lingxing-sync/internal/db"
)

func TestFilterAdAccountsByStoreSIDs(t *testing.T) {
	accounts := []db.AdAccountParams{
		{SID: "100", ProfileID: "p1"},
		{SID: "100", ProfileID: "p2"},
		{SID: "200", ProfileID: "p3"},
	}
	tests := []struct {
		name string
		want []db.AdAccountParams
		sids []string
	}{
		{name: "only selected sid keeps all profiles", sids: []string{"100"}, want: accounts[:2]},
		{name: "unknown sid returns empty", sids: []string{"999"}, want: []db.AdAccountParams{}},
		{name: "caller only invokes for nonempty selection", sids: []string{"100", "200"}, want: accounts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterAdAccountsByStoreSIDs(accounts, tt.sids); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filtered accounts = %#v, want %#v", got, tt.want)
			}
		})
	}
}
