package redmine

import (
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
		},
	}

	tests := []struct {
		name     string
		fieldID  int
		value    string
		want     string
		wantErr  bool
	}{
		{"exact match", 23, "SW Tool", "SW Tool", false},
		{"case-insensitive match", 23, "sw tool", "SW Tool", false},
		{"case-insensitive match upper", 23, "SW TOOL", "SW Tool", false},
		{"case-insensitive match hw", 23, "hw", "HW", false},
		{"no match", 23, "Invalid", "", true},
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
