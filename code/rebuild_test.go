package main

import "testing"

func TestRebuildChannelsForStore(t *testing.T) {

	tests := []struct {
		storeType string
		want      []string
	}{
		{storeType: "SC", want: []string{"sc_fba", "hsa"}},
		{storeType: "", want: []string{"sc_fba", "hsa"}},
		{storeType: "VC", want: []string{"vc"}},
	}
	for _, tc := range tests {
		got, err := rebuildChannelsForStore(tc.storeType)
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

func TestRebuildChannelsForStoreRejectsUnknownType(t *testing.T) {
	if _, err := rebuildChannelsForStore("OTHER"); err == nil {
		t.Fatal("unknown store type should fail loudly")
	}
}
