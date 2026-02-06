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

func TestBuildWorkflowRules(t *testing.T) {
	statuses := []IssueStatus{
		{ID: 1, Name: "New", IsClosed: false},
		{ID: 2, Name: "In Progress", IsClosed: false},
		{ID: 3, Name: "Resolved", IsClosed: false},
		{ID: 5, Name: "Closed", IsClosed: true},
	}

	trackerTransitions := map[int]TrackerTransitionData{
		4: {
			Name: "Bug",
			Transitions: map[int][]IDName{
				1: {{ID: 2, Name: "In Progress"}, {ID: 5, Name: "Closed"}},
				2: {{ID: 3, Name: "Resolved"}, {ID: 1, Name: "New"}},
				3: {{ID: 5, Name: "Closed"}},
			},
		},
	}

	rules := BuildWorkflowRules(statuses, trackerTransitions)

	t.Run("creates tracker entry", func(t *testing.T) {
		tracker, ok := rules.Trackers["4"]
		if !ok {
			t.Fatal("expected tracker 4")
		}
		if tracker.Name != "Bug" {
			t.Fatalf("expected Bug, got %s", tracker.Name)
		}
	})

	t.Run("includes only referenced statuses", func(t *testing.T) {
		tracker := rules.Trackers["4"]
		// Status IDs referenced: 1, 2, 3, 5 (from keys and targets)
		if len(tracker.Statuses) != 4 {
			t.Fatalf("expected 4 statuses, got %d", len(tracker.Statuses))
		}
		if _, ok := tracker.Statuses["1"]; !ok {
			t.Fatal("expected status 1")
		}
		if _, ok := tracker.Statuses["5"]; !ok {
			t.Fatal("expected status 5")
		}
	})

	t.Run("sets IsClosed correctly", func(t *testing.T) {
		tracker := rules.Trackers["4"]
		if tracker.Statuses["1"].IsClosed {
			t.Fatal("New should not be closed")
		}
		if !tracker.Statuses["5"].IsClosed {
			t.Fatal("Closed should be closed")
		}
	})

	t.Run("builds transitions with sorted target IDs", func(t *testing.T) {
		tracker := rules.Trackers["4"]
		from1 := tracker.Transitions["1"]
		if len(from1) != 2 {
			t.Fatalf("expected 2 transitions from status 1, got %d", len(from1))
		}
		// Should be sorted: 2, 5
		if from1[0] != 2 || from1[1] != 5 {
			t.Fatalf("expected [2,5], got %v", from1)
		}
	})

	t.Run("status only in targets is included", func(t *testing.T) {
		// Status 5 (Closed) appears only as a target, never as a from key
		tracker := rules.Trackers["4"]
		if _, ok := tracker.Statuses["5"]; !ok {
			t.Fatal("status 5 should be included even if only in targets")
		}
	})
}

func TestBuildWorkflowRulesEdgeCases(t *testing.T) {
	statuses := []IssueStatus{
		{ID: 1, Name: "New", IsClosed: false},
	}

	t.Run("tracker with no transitions is skipped", func(t *testing.T) {
		transitions := map[int]TrackerTransitionData{
			10: {Name: "Empty", Transitions: map[int][]IDName{}},
		}
		rules := BuildWorkflowRules(statuses, transitions)
		if len(rules.Trackers) != 0 {
			t.Fatalf("expected 0 trackers, got %d", len(rules.Trackers))
		}
	})

	t.Run("empty input returns empty rules", func(t *testing.T) {
		rules := BuildWorkflowRules(nil, nil)
		if rules == nil {
			t.Fatal("expected non-nil rules")
		}
		if len(rules.Trackers) != 0 {
			t.Fatalf("expected 0 trackers, got %d", len(rules.Trackers))
		}
	})

	t.Run("multiple trackers are all included", func(t *testing.T) {
		transitions := map[int]TrackerTransitionData{
			1: {Name: "T1", Transitions: map[int][]IDName{1: {{ID: 1, Name: "New"}}}},
			2: {Name: "T2", Transitions: map[int][]IDName{1: {{ID: 1, Name: "New"}}}},
		}
		rules := BuildWorkflowRules(statuses, transitions)
		if len(rules.Trackers) != 2 {
			t.Fatalf("expected 2 trackers, got %d", len(rules.Trackers))
		}
	})
}

func TestWorkflowMerge(t *testing.T) {
	t.Run("merge adds new trackers", func(t *testing.T) {
		existing := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"1": {Name: "Bug"},
		}}
		other := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"2": {Name: "Task"},
		}}
		existing.Merge(other)
		if len(existing.Trackers) != 2 {
			t.Fatalf("expected 2 trackers, got %d", len(existing.Trackers))
		}
	})

	t.Run("merge overwrites existing trackers", func(t *testing.T) {
		existing := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"1": {Name: "OldBug"},
		}}
		other := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"1": {Name: "NewBug"},
		}}
		existing.Merge(other)
		if existing.Trackers["1"].Name != "NewBug" {
			t.Fatalf("expected NewBug, got %s", existing.Trackers["1"].Name)
		}
	})

	t.Run("merge preserves trackers only in existing", func(t *testing.T) {
		existing := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"1": {Name: "Manual"},
		}}
		other := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"2": {Name: "FromAPI"},
		}}
		existing.Merge(other)
		if existing.Trackers["1"].Name != "Manual" {
			t.Fatal("manual tracker should be preserved")
		}
	})

	t.Run("merge with nil is no-op", func(t *testing.T) {
		existing := &WorkflowRules{Trackers: map[string]WorkflowTracker{
			"1": {Name: "Bug"},
		}}
		existing.Merge(nil)
		if len(existing.Trackers) != 1 {
			t.Fatalf("expected 1 tracker, got %d", len(existing.Trackers))
		}
	})
}

func TestExtractTransitions(t *testing.T) {
	t.Run("extracts status_id changes", func(t *testing.T) {
		journals := []Journal{
			{
				ID: 1,
				Details: []Detail{
					{Property: "attr", Name: "status_id", OldValue: "1", NewValue: "2"},
				},
			},
			{
				ID: 2,
				Details: []Detail{
					{Property: "attr", Name: "status_id", OldValue: "2", NewValue: "5"},
				},
			},
		}
		pairs := extractTransitions(journals)
		if len(pairs) != 2 {
			t.Fatalf("expected 2 pairs, got %d", len(pairs))
		}
		if pairs[0] != [2]int{1, 2} {
			t.Fatalf("expected [1,2], got %v", pairs[0])
		}
		if pairs[1] != [2]int{2, 5} {
			t.Fatalf("expected [2,5], got %v", pairs[1])
		}
	})

	t.Run("ignores non-status details", func(t *testing.T) {
		journals := []Journal{
			{
				ID: 1,
				Details: []Detail{
					{Property: "attr", Name: "assigned_to_id", OldValue: "1", NewValue: "2"},
					{Property: "cf", Name: "1", OldValue: "a", NewValue: "b"},
					{Property: "attr", Name: "status_id", OldValue: "3", NewValue: "4"},
				},
			},
		}
		pairs := extractTransitions(journals)
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != [2]int{3, 4} {
			t.Fatalf("expected [3,4], got %v", pairs[0])
		}
	})

	t.Run("skips invalid values", func(t *testing.T) {
		journals := []Journal{
			{
				ID: 1,
				Details: []Detail{
					{Property: "attr", Name: "status_id", OldValue: "abc", NewValue: "2"},
					{Property: "attr", Name: "status_id", OldValue: "1", NewValue: ""},
				},
			},
		}
		pairs := extractTransitions(journals)
		if len(pairs) != 0 {
			t.Fatalf("expected 0 pairs, got %d", len(pairs))
		}
	})

	t.Run("empty journals returns nil", func(t *testing.T) {
		pairs := extractTransitions(nil)
		if pairs != nil {
			t.Fatalf("expected nil, got %v", pairs)
		}
	})
}

func TestBuildWorkflowRulesDeduplication(t *testing.T) {
	statuses := []IssueStatus{
		{ID: 1, Name: "New", IsClosed: false},
		{ID: 2, Name: "In Progress", IsClosed: false},
		{ID: 3, Name: "Resolved", IsClosed: false},
	}

	// Simulate duplicate transitions (same from→to observed multiple times)
	trackerTransitions := map[int]TrackerTransitionData{
		1: {
			Name: "Bug",
			Transitions: map[int][]IDName{
				1: {
					{ID: 2, Name: "In Progress"},
					{ID: 3, Name: "Resolved"},
					{ID: 2, Name: "In Progress"}, // duplicate
					{ID: 3, Name: "Resolved"},     // duplicate
					{ID: 2, Name: "In Progress"}, // triple
				},
			},
		},
	}

	rules := BuildWorkflowRules(statuses, trackerTransitions)
	tracker := rules.Trackers["1"]
	from1 := tracker.Transitions["1"]

	if len(from1) != 2 {
		t.Fatalf("expected 2 deduplicated transitions, got %d: %v", len(from1), from1)
	}
	// Should be sorted: 2, 3
	if from1[0] != 2 || from1[1] != 3 {
		t.Fatalf("expected [2,3], got %v", from1)
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
