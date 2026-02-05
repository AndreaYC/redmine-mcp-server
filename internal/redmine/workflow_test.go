package redmine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflowRules(t *testing.T) {
	// Non-existent file returns nil, nil
	rules, err := LoadWorkflowRules("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if rules != nil {
		t.Fatal("expected nil rules for missing file")
	}

	// Valid JSON file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "workflow.json")
	data := `{"trackers":{"4":{"name":"Bug","statuses":{"33":{"name":"New","is_closed":false},"9":{"name":"Clarifying","is_closed":false}},"transitions":{"33":[9],"9":[]}}}}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	rules, err = LoadWorkflowRules(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules == nil {
		t.Fatal("expected non-nil rules")
	}
	if len(rules.Trackers) != 1 {
		t.Fatalf("expected 1 tracker, got %d", len(rules.Trackers))
	}
	if rules.Trackers["4"].Name != "Bug" {
		t.Fatalf("expected Bug, got %s", rules.Trackers["4"].Name)
	}

	// Invalid JSON file
	badPath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(badPath, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadWorkflowRules(badPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateTransition(t *testing.T) {
	rules := &WorkflowRules{
		Trackers: map[string]WorkflowTracker{
			"4": {
				Name: "Bug",
				Statuses: map[string]WorkflowStatus{
					"33": {Name: "New", IsClosed: false},
					"9":  {Name: "Clarifying", IsClosed: false},
					"20": {Name: "Fixing", IsClosed: false},
					"6":  {Name: "Rejected", IsClosed: true},
				},
				Transitions: map[string][]int{
					"33": {9, 20, 6},
					"9":  {20, 6},
					"20": {9, 6},
					"6":  {9},
				},
			},
			"32": {
				Name: "SW_Task",
				Statuses: map[string]WorkflowStatus{
					"5": {Name: "Closed", IsClosed: true},
				},
				Transitions: map[string][]int{
					"5": {},
				},
			},
		},
	}

	tests := []struct {
		name      string
		rules     *WorkflowRules
		trackerID int
		fromID    int
		toID      int
		wantErr   bool
	}{
		{"allowed transition", rules, 4, 33, 9, false},
		{"allowed transition 2", rules, 4, 6, 9, false},
		{"disallowed transition", rules, 4, 6, 20, true},
		{"disallowed - no exits", rules, 32, 5, 33, true},
		{"unknown tracker passes through", rules, 999, 33, 9, false},
		{"unknown from_status passes through", rules, 4, 999, 9, false},
		{"nil rules passes through", nil, 4, 33, 9, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rules.ValidateTransition(tt.trackerID, tt.fromID, tt.toID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateTransitionErrorMessage(t *testing.T) {
	rules := &WorkflowRules{
		Trackers: map[string]WorkflowTracker{
			"4": {
				Name: "Bug",
				Statuses: map[string]WorkflowStatus{
					"33": {Name: "New", IsClosed: false},
					"9":  {Name: "Clarifying", IsClosed: false},
					"20": {Name: "Fixing", IsClosed: false},
					"6":  {Name: "Rejected", IsClosed: true},
				},
				Transitions: map[string][]int{
					"6": {9},
				},
			},
		},
	}

	err := rules.ValidateTransition(4, 6, 20)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	// Should mention status names and allowed targets
	if !contains(msg, "Rejected") || !contains(msg, "Fixing") || !contains(msg, "Clarifying") {
		t.Fatalf("error message should include status names, got: %s", msg)
	}
}

func TestValidateTransitionNoExitsMessage(t *testing.T) {
	rules := &WorkflowRules{
		Trackers: map[string]WorkflowTracker{
			"32": {
				Name: "SW_Task",
				Statuses: map[string]WorkflowStatus{
					"5":  {Name: "Closed", IsClosed: true},
					"33": {Name: "New", IsClosed: false},
				},
				Transitions: map[string][]int{
					"5": {},
				},
			},
		},
	}

	err := rules.ValidateTransition(32, 5, 33)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !contains(msg, "no transitions allowed") {
		t.Fatalf("error message should mention no transitions allowed, got: %s", msg)
	}
}

func TestGetAllowedStatuses(t *testing.T) {
	rules := &WorkflowRules{
		Trackers: map[string]WorkflowTracker{
			"4": {
				Name: "Bug",
				Statuses: map[string]WorkflowStatus{
					"33": {Name: "New", IsClosed: false},
					"9":  {Name: "Clarifying", IsClosed: false},
					"20": {Name: "Fixing", IsClosed: false},
					"6":  {Name: "Rejected", IsClosed: true},
				},
				Transitions: map[string][]int{
					"33": {9, 20, 6},
				},
			},
		},
	}

	// Known tracker and status
	allowed := rules.GetAllowedStatuses(4, 33)
	if len(allowed) != 3 {
		t.Fatalf("expected 3 allowed statuses, got %d", len(allowed))
	}
	if allowed[0].Name != "Clarifying" || allowed[1].Name != "Fixing" || allowed[2].Name != "Rejected" {
		t.Fatalf("unexpected allowed statuses: %v", allowed)
	}

	// Unknown tracker returns nil
	if got := rules.GetAllowedStatuses(999, 33); got != nil {
		t.Fatalf("expected nil for unknown tracker, got %v", got)
	}

	// Unknown status returns nil
	if got := rules.GetAllowedStatuses(4, 999); got != nil {
		t.Fatalf("expected nil for unknown status, got %v", got)
	}

	// Nil rules returns nil
	var nilRules *WorkflowRules
	if got := nilRules.GetAllowedStatuses(4, 33); got != nil {
		t.Fatalf("expected nil for nil rules, got %v", got)
	}
}

func TestGetTrackerStatuses(t *testing.T) {
	rules := &WorkflowRules{
		Trackers: map[string]WorkflowTracker{
			"4": {
				Name: "Bug",
				Statuses: map[string]WorkflowStatus{
					"33": {Name: "New", IsClosed: false},
					"9":  {Name: "Clarifying", IsClosed: false},
				},
				Transitions: map[string][]int{},
			},
		},
	}

	statuses := rules.GetTrackerStatuses(4)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	// Unknown tracker
	if got := rules.GetTrackerStatuses(999); got != nil {
		t.Fatalf("expected nil for unknown tracker, got %v", got)
	}

	// Nil rules
	var nilRules *WorkflowRules
	if got := nilRules.GetTrackerStatuses(4); got != nil {
		t.Fatalf("expected nil for nil rules, got %v", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
