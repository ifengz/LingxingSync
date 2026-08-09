package server

import (
	"testing"

	"lingxing-sync/internal/config"
)

func TestCatalogAccountEnabledRecognizesExactAndLegacyEndpoints(t *testing.T) {
	entry, err := config.FindCatalogEntry("vc_stores")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}

	tests := []struct {
		name      string
		endpoints []config.Endpoint
		want      bool
	}{
		{
			name: "exact generated name",
			endpoints: []config.Endpoint{
				entry.ToEndpoint("sc_us_1"),
			},
			want: true,
		},
		{
			name: "legacy name with same path",
			endpoints: []config.Endpoint{
				{Name: "vc_stores", Account: "sc_us_1", Path: entry.Path},
			},
			want: true,
		},
		{
			name: "not enabled",
			endpoints: []config.Endpoint{
				{Name: "sc_stores", Account: "sc_us_1", Path: "/erp/sc/data/seller/lists"},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Accounts:  []config.Account{{ID: "sc_us_1"}},
				Endpoints: tc.endpoints,
			}
			if got := catalogAccountEnabled(cfg, entry, "sc_us_1"); got != tc.want {
				t.Fatalf("catalogAccountEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
