package epinput

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestWriteEPJSONPreservesSharedModel(t *testing.T) {
	for _, metadata := range []map[string]any{nil, {"idf_max_extensible_fields": 4}, {"idf_order": json.Number("19")}} {
		model := &Model{Objects: []InputObject{{
			Type: "Zone", Name: "Office", SourceIndex: 6, Metadata: metadata,
			Fields: []Field{{Key: "direction_of_relative_north", Value: json.Number("0")}},
		}}}
		before, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		wantOrder := `"idf_order": 7`
		if _, ok := metadata["idf_order"]; ok {
			wantOrder = `"idf_order": 19`
		}
		var workers sync.WaitGroup
		for range 8 {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for range 10 {
					output, err := WriteEPJSON(model)
					if err != nil || !strings.Contains(output, wantOrder) {
						t.Errorf("WriteEPJSON() order = %q, want %s (error %v)", output, wantOrder, err)
					}
					// Cache users may serialize the same input while exporting it.
					if _, err := json.Marshal(model); err != nil {
						t.Error(err)
					}
				}
			}()
		}
		workers.Wait()
		after, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatalf("EPJSON export mutated the model:\nbefore %s\nafter %s", before, after)
		}
	}
}
