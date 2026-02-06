package redmine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCustomFieldRules(t *testing.T) {
	// Non-existent file returns nil, nil
	rules, err := LoadCustomFieldRules("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if rules != nil {
		t.Fatal("expected nil rules for missing file")
	}

	// Valid JSON file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "rules.json")
	data := `{"fields":{"23":{"name":"Component","values":["SW Tool","HW"]}}}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	rules, err = LoadCustomFieldRules(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules == nil {
		t.Fatal("expected non-nil rules")
	}
	if len(rules.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(rules.Fields))
	}
	if rules.Fields["23"].Name != "Component" {
		t.Fatalf("expected Component, got %s", rules.Fields["23"].Name)
	}
}

func TestValidateValue(t *testing.T) {
	rules := &CustomFieldRules{
		Fields: map[string]CustomFieldRule{
			"23": {
				Name:   "Component",
				Values: []string{"SW Tool", "HW", "FPGA", "(none)"},
			},
			"27": {
				Name:               "HW Version",
				RequiredByTrackers: []int{4, 22},
				// No Values — free-text field
			},
		},
	}

	tests := []struct {
		name    string
		fieldID int
		value   string
		want    string
		wantErr bool
	}{
		{"exact match", 23, "SW Tool", "SW Tool", false},
		{"case-insensitive match", 23, "sw tool", "SW Tool", false},
		{"case-insensitive match upper", 23, "SW TOOL", "SW Tool", false},
		{"case-insensitive match hw", 23, "hw", "HW", false},
		{"no match", 23, "Invalid", "", true},
		{"free-text field passes through", 27, "any value", "any value", false},
		{"unknown field passes through", 999, "anything", "anything", false},
		{"nil rules passes through", 23, "anything", "anything", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rules
			if tt.name == "nil rules passes through" {
				r = nil
			}
			got, err := r.ValidateValue(tt.fieldID, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateValues(t *testing.T) {
	rules := &CustomFieldRules{
		Fields: map[string]CustomFieldRule{
			"23": {
				Name:   "Component",
				Values: []string{"SW Tool", "HW"},
			},
		},
	}

	// All valid
	result, err := rules.ValidateValues(23, []string{"sw tool", "hw"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0] != "SW Tool" || result[1] != "HW" {
		t.Fatalf("unexpected result: %v", result)
	}

	// One invalid
	_, err = rules.ValidateValues(23, []string{"sw tool", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid value")
	}
}

func TestGetRequiredFieldsForTracker(t *testing.T) {
	rules := &CustomFieldRules{
		Fields: map[string]CustomFieldRule{
			"23":  {Name: "Component", Values: []string{"HW", "SW"}},
			"223": {Name: "SW_Category", Values: []string{"Debug", "Other"}, RequiredByTrackers: []int{32}},
			"27":  {Name: "HW Version", RequiredByTrackers: []int{4, 22}},
			"126": {Name: "Bug Create After MP", Values: []string{"1", "0"}, RequiredByTrackers: []int{4}},
		},
	}

	t.Run("returns fields required for matching tracker", func(t *testing.T) {
		got := rules.GetRequiredFieldsForTracker(4)
		if len(got) != 2 {
			t.Fatalf("expected 2 required fields for tracker 4, got %d: %v", len(got), got)
		}
		if got[27] != "HW Version" {
			t.Errorf("expected HW Version for ID 27, got %q", got[27])
		}
		if got[126] != "Bug Create After MP" {
			t.Errorf("expected Bug Create After MP for ID 126, got %q", got[126])
		}
	})

	t.Run("returns only fields for specific tracker", func(t *testing.T) {
		got := rules.GetRequiredFieldsForTracker(32)
		if len(got) != 1 {
			t.Fatalf("expected 1 required field for tracker 32, got %d: %v", len(got), got)
		}
		if got[223] != "SW_Category" {
			t.Errorf("expected SW_Category for ID 223, got %q", got[223])
		}
	})

	t.Run("returns empty map for tracker with no required fields", func(t *testing.T) {
		got := rules.GetRequiredFieldsForTracker(999)
		if len(got) != 0 {
			t.Fatalf("expected 0 required fields for tracker 999, got %d", len(got))
		}
	})

	t.Run("trackerID 0 returns empty map", func(t *testing.T) {
		got := rules.GetRequiredFieldsForTracker(0)
		if len(got) != 0 {
			t.Fatalf("expected 0 required fields for trackerID 0, got %d", len(got))
		}
	})

	t.Run("nil rules returns empty map", func(t *testing.T) {
		var nilRules *CustomFieldRules
		got := nilRules.GetRequiredFieldsForTracker(4)
		if len(got) != 0 {
			t.Fatalf("expected 0 required fields, got %d", len(got))
		}
	})
}

func TestGenerateCustomFieldRules(t *testing.T) {
	fields := []CustomFieldDefinitionFull{
		{
			ID:             23,
			Name:           "Component",
			CustomizedType: "issue",
			IsRequired:     false,
			PossibleValues: []struct {
				Value string `json:"value"`
			}{
				{Value: "SW Tool"},
				{Value: "HW"},
			},
			Trackers: []IDName{{ID: 4, Name: "Bug"}, {ID: 22, Name: "Task"}},
		},
		{
			ID:             27,
			Name:           "HW Version",
			CustomizedType: "issue",
			IsRequired:     true,
			Trackers:       []IDName{{ID: 4, Name: "Bug"}},
		},
		{
			ID:             50,
			Name:           "Department",
			CustomizedType: "user", // not issue — should be filtered out
			IsRequired:     true,
		},
		{
			ID:             99,
			Name:           "Free Text",
			CustomizedType: "issue",
			IsRequired:     false,
		},
		{
			ID:             100,
			Name:           "Global Required",
			CustomizedType: "issue",
			IsRequired:     true,
			Trackers:       nil, // required but no specific trackers
		},
	}

	rules := GenerateCustomFieldRules(fields)

	t.Run("filters out non-issue fields", func(t *testing.T) {
		if _, ok := rules.Fields["50"]; ok {
			t.Fatal("user-type field should not be included")
		}
	})

	t.Run("includes issue fields", func(t *testing.T) {
		if len(rules.Fields) != 4 {
			t.Fatalf("expected 4 fields, got %d", len(rules.Fields))
		}
	})

	t.Run("converts possible_values to values", func(t *testing.T) {
		rule := rules.Fields["23"]
		if len(rule.Values) != 2 || rule.Values[0] != "SW Tool" || rule.Values[1] != "HW" {
			t.Fatalf("expected [SW Tool, HW], got %v", rule.Values)
		}
	})

	t.Run("sets required_by_trackers when required and has trackers", func(t *testing.T) {
		rule := rules.Fields["27"]
		if len(rule.RequiredByTrackers) != 1 || rule.RequiredByTrackers[0] != 4 {
			t.Fatalf("expected [4], got %v", rule.RequiredByTrackers)
		}
	})

	t.Run("no required_by_trackers when not required", func(t *testing.T) {
		rule := rules.Fields["23"]
		if rule.RequiredByTrackers != nil {
			t.Fatalf("expected nil RequiredByTrackers, got %v", rule.RequiredByTrackers)
		}
	})

	t.Run("no required_by_trackers when required but no trackers", func(t *testing.T) {
		rule := rules.Fields["100"]
		if rule.RequiredByTrackers != nil {
			t.Fatalf("expected nil RequiredByTrackers, got %v", rule.RequiredByTrackers)
		}
	})

	t.Run("free-text field has empty values", func(t *testing.T) {
		rule := rules.Fields["99"]
		if len(rule.Values) != 0 {
			t.Fatalf("expected empty values, got %v", rule.Values)
		}
	})
}

func TestMerge(t *testing.T) {
	t.Run("merge adds new fields", func(t *testing.T) {
		existing := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"1": {Name: "A", Values: []string{"x"}},
		}}
		generated := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"2": {Name: "B", Values: []string{"y"}},
		}}
		existing.Merge(generated)
		if len(existing.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(existing.Fields))
		}
	})

	t.Run("merge overwrites existing fields", func(t *testing.T) {
		existing := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"1": {Name: "A", Values: []string{"old"}},
		}}
		generated := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"1": {Name: "A", Values: []string{"new"}},
		}}
		existing.Merge(generated)
		if existing.Fields["1"].Values[0] != "new" {
			t.Fatalf("expected 'new', got %q", existing.Fields["1"].Values[0])
		}
	})

	t.Run("merge preserves fields only in existing", func(t *testing.T) {
		existing := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"1": {Name: "Manual", Values: []string{"x"}},
		}}
		generated := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"2": {Name: "FromAPI", Values: []string{"y"}},
		}}
		existing.Merge(generated)
		if existing.Fields["1"].Name != "Manual" {
			t.Fatal("manual field should be preserved")
		}
	})

	t.Run("merge with nil is no-op", func(t *testing.T) {
		existing := &CustomFieldRules{Fields: map[string]CustomFieldRule{
			"1": {Name: "A"},
		}}
		existing.Merge(nil)
		if len(existing.Fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(existing.Fields))
		}
	})
}

func TestRequiredByTrackersJSONDeserialization(t *testing.T) {
	t.Run("required_by_trackers is deserialized", func(t *testing.T) {
		data := `{"fields":{"223":{"name":"SW_Category","required_by_trackers":[32],"values":["Debug"]}}}`
		var rules CustomFieldRules
		if err := json.Unmarshal([]byte(data), &rules); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		rbt := rules.Fields["223"].RequiredByTrackers
		if len(rbt) != 1 || rbt[0] != 32 {
			t.Fatalf("expected RequiredByTrackers=[32], got %v", rbt)
		}
	})

	t.Run("multiple trackers are deserialized", func(t *testing.T) {
		data := `{"fields":{"27":{"name":"HW Version","required_by_trackers":[4,22]}}}`
		var rules CustomFieldRules
		if err := json.Unmarshal([]byte(data), &rules); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		rbt := rules.Fields["27"].RequiredByTrackers
		if len(rbt) != 2 || rbt[0] != 4 || rbt[1] != 22 {
			t.Fatalf("expected RequiredByTrackers=[4,22], got %v", rbt)
		}
	})

	t.Run("omitted required_by_trackers defaults to nil", func(t *testing.T) {
		data := `{"fields":{"23":{"name":"Component","values":["HW"]}}}`
		var rules CustomFieldRules
		if err := json.Unmarshal([]byte(data), &rules); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if rules.Fields["23"].RequiredByTrackers != nil {
			t.Fatalf("expected nil RequiredByTrackers, got %v", rules.Fields["23"].RequiredByTrackers)
		}
	})
}
