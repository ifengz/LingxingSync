package rebuild

import (
	"context"
	"testing"
	"time"
)

func TestChannelsForStore(t *testing.T) {
	tests := []struct {
		storeType string
		want      []string
	}{
		{storeType: "SC", want: []string{"sc_fba", "hsa"}},
		{storeType: "", want: []string{"sc_fba", "hsa"}},
		{storeType: "sc", want: []string{"sc_fba", "hsa"}},
		{storeType: "VC", want: []string{"vc"}},
	}
	for _, tc := range tests {
		got, err := ChannelsForStore(tc.storeType)
		if err != nil {
			t.Fatalf("store type %q: unexpected error: %v", tc.storeType, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("store type %q: channels=%v, want %v", tc.storeType, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("store type %q: channels=%v, want %v", tc.storeType, got, tc.want)
			}
		}
	}
}

func TestChannelsForStoreRejectsUnknownType(t *testing.T) {
	if _, err := ChannelsForStore("OTHER"); err == nil {
		t.Fatal("unknown store type should fail loudly")
	}
}

func TestParseDate(t *testing.T) {
	got, err := ParseDate("2026-08-31", "-rebuild-date-from")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Format("2006-01-02") != "2026-08-31" {
		t.Fatalf("got %s, want 2026-08-31", got.Format("2006-01-02"))
	}
	if _, err := ParseDate("not-a-date", "-rebuild-date-from"); err == nil {
		t.Fatal("invalid date should fail loudly")
	}
}

func TestRunListingDailyRejectsInvalidRange(t *testing.T) {
	if _, err := RunListingDaily(context.Background(), nil, nil, "", "", time.Time{}, time.Time{}, nil); err == nil {
		t.Fatal("nil db/config should fail loudly")
	}
	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if _, err := RunListingDaily(context.Background(), nil, nil, "", "", from, to, nil); err == nil {
		t.Fatal("from-after-to should fail loudly")
	}
}