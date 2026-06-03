package dictionary

import "testing"

func TestLoad(t *testing.T) {
	entries, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("Load() returned no entries")
	}
	for i, e := range entries {
		if e.Key == "" || e.Value == "" {
			t.Errorf("entries[%d] has empty Key or Value: %+v", i, e)
		}
	}
}
