package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

// --- TestResolveDatePeriod ---

func TestResolveDatePeriod(t *testing.T) {
	t.Run("this_week returns Monday to Sunday of current week", func(t *testing.T) {
		from, to := resolveDatePeriod("this_week")
		if from == "" || to == "" {
			t.Fatal("expected non-empty from/to for this_week")
		}

		fromDate, err := time.Parse("2006-01-02", from)
		if err != nil {
			t.Fatalf("invalid from date format: %v", err)
		}
		toDate, err := time.Parse("2006-01-02", to)
		if err != nil {
			t.Fatalf("invalid to date format: %v", err)
		}

		// from must be Monday
		if fromDate.Weekday() != time.Monday {
			t.Errorf("expected from to be Monday, got %s", fromDate.Weekday())
		}
		// to must be Sunday
		if toDate.Weekday() != time.Sunday {
			t.Errorf("expected to to be Sunday, got %s", toDate.Weekday())
		}
		// Span must be exactly 6 days
		diff := toDate.Sub(fromDate)
		if diff != 6*24*time.Hour {
			t.Errorf("expected 6-day span, got %v", diff)
		}
		// from must be in the current week
		now := time.Now()
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		expectedStart := now.AddDate(0, 0, -weekday+1)
		expectedFrom := expectedStart.Format("2006-01-02")
		if from != expectedFrom {
			t.Errorf("expected from=%s, got %s", expectedFrom, from)
		}
	})

	t.Run("last_week returns Monday to Sunday of previous week", func(t *testing.T) {
		from, to := resolveDatePeriod("last_week")
		if from == "" || to == "" {
			t.Fatal("expected non-empty from/to for last_week")
		}

		fromDate, err := time.Parse("2006-01-02", from)
		if err != nil {
			t.Fatalf("invalid from date format: %v", err)
		}
		toDate, err := time.Parse("2006-01-02", to)
		if err != nil {
			t.Fatalf("invalid to date format: %v", err)
		}

		// from must be Monday
		if fromDate.Weekday() != time.Monday {
			t.Errorf("expected from to be Monday, got %s", fromDate.Weekday())
		}
		// to must be Sunday
		if toDate.Weekday() != time.Sunday {
			t.Errorf("expected to to be Sunday, got %s", toDate.Weekday())
		}
		// Span must be exactly 6 days
		diff := toDate.Sub(fromDate)
		if diff != 6*24*time.Hour {
			t.Errorf("expected 6-day span, got %v", diff)
		}
		// The last_week Sunday must be before this_week Monday
		thisFrom, _ := resolveDatePeriod("this_week")
		thisFromDate, _ := time.Parse("2006-01-02", thisFrom)
		if !toDate.Before(thisFromDate) {
			t.Errorf("last_week end (%s) should be before this_week start (%s)", to, thisFrom)
		}
		// And the difference between this Monday and last Monday should be exactly 7 days
		lastToThisDiff := thisFromDate.Sub(fromDate)
		if lastToThisDiff != 7*24*time.Hour {
			t.Errorf("expected 7-day gap between last_week start and this_week start, got %v", lastToThisDiff)
		}
	})

	t.Run("this_month returns first to last day of current month", func(t *testing.T) {
		from, to := resolveDatePeriod("this_month")
		if from == "" || to == "" {
			t.Fatal("expected non-empty from/to for this_month")
		}

		fromDate, err := time.Parse("2006-01-02", from)
		if err != nil {
			t.Fatalf("invalid from date format: %v", err)
		}
		toDate, err := time.Parse("2006-01-02", to)
		if err != nil {
			t.Fatalf("invalid to date format: %v", err)
		}

		now := time.Now()
		// from must be first day of current month
		if fromDate.Day() != 1 {
			t.Errorf("expected from day=1, got %d", fromDate.Day())
		}
		if fromDate.Month() != now.Month() {
			t.Errorf("expected from month=%s, got %s", now.Month(), fromDate.Month())
		}
		if fromDate.Year() != now.Year() {
			t.Errorf("expected from year=%d, got %d", now.Year(), fromDate.Year())
		}
		// to must be last day of current month
		nextDay := toDate.AddDate(0, 0, 1)
		if nextDay.Day() != 1 {
			t.Errorf("expected to to be last day of month, but next day is %d", nextDay.Day())
		}
		// from <= to
		if fromDate.After(toDate) {
			t.Errorf("from (%s) should not be after to (%s)", from, to)
		}
	})

	t.Run("last_month returns first to last day of previous month", func(t *testing.T) {
		from, to := resolveDatePeriod("last_month")
		if from == "" || to == "" {
			t.Fatal("expected non-empty from/to for last_month")
		}

		fromDate, err := time.Parse("2006-01-02", from)
		if err != nil {
			t.Fatalf("invalid from date format: %v", err)
		}
		toDate, err := time.Parse("2006-01-02", to)
		if err != nil {
			t.Fatalf("invalid to date format: %v", err)
		}

		now := time.Now()
		expectedMonth := now.Month() - 1
		expectedYear := now.Year()
		if expectedMonth == 0 {
			expectedMonth = 12
			expectedYear--
		}

		// from must be first day of previous month
		if fromDate.Day() != 1 {
			t.Errorf("expected from day=1, got %d", fromDate.Day())
		}
		if fromDate.Month() != expectedMonth {
			t.Errorf("expected from month=%s, got %s", expectedMonth, fromDate.Month())
		}
		if fromDate.Year() != expectedYear {
			t.Errorf("expected from year=%d, got %d", expectedYear, fromDate.Year())
		}
		// to must be last day of previous month
		nextDay := toDate.AddDate(0, 0, 1)
		if nextDay.Day() != 1 {
			t.Errorf("expected to to be last day of month, but next day is %d", nextDay.Day())
		}
		// to's month should still be the previous month
		if toDate.Month() != expectedMonth {
			t.Errorf("expected to month=%s, got %s", expectedMonth, toDate.Month())
		}
		// from <= to
		if fromDate.After(toDate) {
			t.Errorf("from (%s) should not be after to (%s)", from, to)
		}
	})

	t.Run("empty string returns empty strings", func(t *testing.T) {
		from, to := resolveDatePeriod("")
		if from != "" || to != "" {
			t.Errorf("expected empty strings for empty period, got from=%q to=%q", from, to)
		}
	})

	t.Run("invalid period returns empty strings", func(t *testing.T) {
		from, to := resolveDatePeriod("invalid")
		if from != "" || to != "" {
			t.Errorf("expected empty strings for invalid period, got from=%q to=%q", from, to)
		}
	})
}

// --- TestFormatIssue ---

func TestFormatIssue(t *testing.T) {
	issue := redmine.Issue{
		ID: 42,
		Project: redmine.IDName{ID: 1, Name: "Test Project"},
		Tracker: redmine.IDName{ID: 2, Name: "Bug"},
		Status:  redmine.IDName{ID: 3, Name: "New"},
		Priority: redmine.IDName{ID: 4, Name: "High"},
		Author:  redmine.IDName{ID: 5, Name: "Alice"},
		Subject: "Fix login bug",
		AssignedTo: &redmine.IDName{ID: 6, Name: "Bob"},
		StartDate: "2025-01-01",
		DueDate:   "2025-01-31",
		ClosedOn:  "2025-02-01",
		CreatedOn: "2024-12-01T10:00:00Z",
		UpdatedOn: "2025-01-15T14:30:00Z",
	}

	result := formatIssue(issue)

	// Check top-level fields
	if result["id"] != 42 {
		t.Errorf("expected id=42, got %v", result["id"])
	}
	if result["subject"] != "Fix login bug" {
		t.Errorf("expected subject='Fix login bug', got %v", result["subject"])
	}
	if result["created_on"] != "2024-12-01T10:00:00Z" {
		t.Errorf("expected created_on, got %v", result["created_on"])
	}
	if result["updated_on"] != "2025-01-15T14:30:00Z" {
		t.Errorf("expected updated_on, got %v", result["updated_on"])
	}

	// Check nested IDName fields
	projectMap, ok := result["project"].(map[string]any)
	if !ok {
		t.Fatal("expected project to be map[string]any")
	}
	if projectMap["id"] != 1 || projectMap["name"] != "Test Project" {
		t.Errorf("unexpected project: %v", projectMap)
	}

	trackerMap, ok := result["tracker"].(map[string]any)
	if !ok {
		t.Fatal("expected tracker to be map[string]any")
	}
	if trackerMap["id"] != 2 || trackerMap["name"] != "Bug" {
		t.Errorf("unexpected tracker: %v", trackerMap)
	}

	statusMap, ok := result["status"].(map[string]any)
	if !ok {
		t.Fatal("expected status to be map[string]any")
	}
	if statusMap["id"] != 3 || statusMap["name"] != "New" {
		t.Errorf("unexpected status: %v", statusMap)
	}

	priorityMap, ok := result["priority"].(map[string]any)
	if !ok {
		t.Fatal("expected priority to be map[string]any")
	}
	if priorityMap["id"] != 4 || priorityMap["name"] != "High" {
		t.Errorf("unexpected priority: %v", priorityMap)
	}

	authorMap, ok := result["author"].(map[string]any)
	if !ok {
		t.Fatal("expected author to be map[string]any")
	}
	if authorMap["id"] != 5 || authorMap["name"] != "Alice" {
		t.Errorf("unexpected author: %v", authorMap)
	}

	// Check assigned_to (pointer field)
	assignedMap, ok := result["assigned_to"].(map[string]any)
	if !ok {
		t.Fatal("expected assigned_to to be map[string]any")
	}
	if assignedMap["id"] != 6 || assignedMap["name"] != "Bob" {
		t.Errorf("unexpected assigned_to: %v", assignedMap)
	}

	// Check optional date fields
	if result["start_date"] != "2025-01-01" {
		t.Errorf("expected start_date='2025-01-01', got %v", result["start_date"])
	}
	if result["due_date"] != "2025-01-31" {
		t.Errorf("expected due_date='2025-01-31', got %v", result["due_date"])
	}
	if result["closed_on"] != "2025-02-01" {
		t.Errorf("expected closed_on='2025-02-01', got %v", result["closed_on"])
	}
}

func TestFormatIssue_NilAssignedTo(t *testing.T) {
	issue := redmine.Issue{
		ID:      1,
		Project: redmine.IDName{ID: 1, Name: "P"},
		Tracker: redmine.IDName{ID: 1, Name: "T"},
		Status:  redmine.IDName{ID: 1, Name: "S"},
		Priority: redmine.IDName{ID: 1, Name: "Pr"},
		Author:  redmine.IDName{ID: 1, Name: "A"},
		Subject: "Test",
	}

	result := formatIssue(issue)

	if _, exists := result["assigned_to"]; exists {
		t.Error("expected assigned_to to be absent when nil")
	}
	if _, exists := result["start_date"]; exists {
		t.Error("expected start_date to be absent when empty")
	}
	if _, exists := result["due_date"]; exists {
		t.Error("expected due_date to be absent when empty")
	}
	if _, exists := result["closed_on"]; exists {
		t.Error("expected closed_on to be absent when empty")
	}
}

// --- TestFormatIssueDetail ---

func TestFormatIssueDetail(t *testing.T) {
	issue := redmine.Issue{
		ID:          100,
		Project:     redmine.IDName{ID: 1, Name: "MyProject"},
		Tracker:     redmine.IDName{ID: 2, Name: "Feature"},
		Status:      redmine.IDName{ID: 3, Name: "In Progress"},
		Priority:    redmine.IDName{ID: 4, Name: "Normal"},
		Author:      redmine.IDName{ID: 5, Name: "Carol"},
		AssignedTo:  &redmine.IDName{ID: 6, Name: "Dave"},
		Subject:     "Implement feature X",
		Description: "Detailed description of feature X.",
		DoneRatio:   50,
		CreatedOn:   "2025-01-01T00:00:00Z",
		UpdatedOn:   "2025-01-10T00:00:00Z",
		Parent: &struct {
			ID int `json:"id"`
		}{ID: 99},
		CustomFields: []redmine.CustomField{
			{ID: 23, Name: "Component", Value: "SW Tool"},
			{ID: 223, Name: "SW_Category", Value: "Firmware"},
		},
		Journals: []redmine.Journal{
			{
				ID:        1001,
				User:      redmine.IDName{ID: 5, Name: "Carol"},
				Notes:     "Started working on this.",
				CreatedOn: "2025-01-02T08:00:00Z",
			},
		},
		Watchers: []redmine.IDName{
			{ID: 7, Name: "Eve"},
			{ID: 8, Name: "Frank"},
		},
		Relations: []redmine.Relation{
			{ID: 500, IssueID: 100, IssueToID: 101, RelationType: "relates"},
		},
		AllowedStatuses: []redmine.IDName{
			{ID: 3, Name: "In Progress"},
			{ID: 5, Name: "Closed"},
		},
		Attachments: []redmine.Attachment{
			{
				ID:          200,
				Filename:    "spec.pdf",
				Filesize:    1024,
				ContentType: "application/pdf",
				Description: "Specification document",
				CreatedOn:   "2025-01-01T01:00:00Z",
				Author:      redmine.IDName{ID: 5, Name: "Carol"},
			},
		},
	}

	result := formatIssueDetail(issue)

	// Check fields inherited from formatIssue
	if result["id"] != 100 {
		t.Errorf("expected id=100, got %v", result["id"])
	}
	if result["subject"] != "Implement feature X" {
		t.Errorf("unexpected subject: %v", result["subject"])
	}

	// Check fields added by formatIssueDetail
	if result["description"] != "Detailed description of feature X." {
		t.Errorf("unexpected description: %v", result["description"])
	}
	if result["done_ratio"] != 50 {
		t.Errorf("expected done_ratio=50, got %v", result["done_ratio"])
	}
	if result["parent_issue_id"] != 99 {
		t.Errorf("expected parent_issue_id=99, got %v", result["parent_issue_id"])
	}

	// Custom fields
	cfMap, ok := result["custom_fields"].(map[string]any)
	if !ok {
		t.Fatal("expected custom_fields to be map[string]any")
	}
	if cfMap["Component"] != "SW Tool" {
		t.Errorf("expected Component='SW Tool', got %v", cfMap["Component"])
	}
	if cfMap["SW_Category"] != "Firmware" {
		t.Errorf("expected SW_Category='Firmware', got %v", cfMap["SW_Category"])
	}

	// Journals
	journals, ok := result["journals"].([]map[string]any)
	if !ok {
		t.Fatal("expected journals to be []map[string]any")
	}
	if len(journals) != 1 {
		t.Fatalf("expected 1 journal, got %d", len(journals))
	}
	if journals[0]["id"] != 1001 {
		t.Errorf("expected journal id=1001, got %v", journals[0]["id"])
	}
	if journals[0]["user"] != "Carol" {
		t.Errorf("expected journal user='Carol', got %v", journals[0]["user"])
	}
	if journals[0]["notes"] != "Started working on this." {
		t.Errorf("unexpected journal notes: %v", journals[0]["notes"])
	}

	// Watchers
	watchers, ok := result["watchers"].([]map[string]any)
	if !ok {
		t.Fatal("expected watchers to be []map[string]any")
	}
	if len(watchers) != 2 {
		t.Fatalf("expected 2 watchers, got %d", len(watchers))
	}
	if watchers[0]["name"] != "Eve" {
		t.Errorf("expected first watcher name='Eve', got %v", watchers[0]["name"])
	}
	if watchers[1]["name"] != "Frank" {
		t.Errorf("expected second watcher name='Frank', got %v", watchers[1]["name"])
	}

	// Relations
	relations, ok := result["relations"].([]map[string]any)
	if !ok {
		t.Fatal("expected relations to be []map[string]any")
	}
	if len(relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(relations))
	}
	if relations[0]["id"] != 500 {
		t.Errorf("expected relation id=500, got %v", relations[0]["id"])
	}
	if relations[0]["relation_type"] != "relates" {
		t.Errorf("expected relation_type='relates', got %v", relations[0]["relation_type"])
	}
	if relations[0]["issue_id"] != 100 {
		t.Errorf("expected relation issue_id=100, got %v", relations[0]["issue_id"])
	}
	if relations[0]["issue_to_id"] != 101 {
		t.Errorf("expected relation issue_to_id=101, got %v", relations[0]["issue_to_id"])
	}

	// Allowed statuses
	allowedStatuses, ok := result["allowed_statuses"].([]map[string]any)
	if !ok {
		t.Fatal("expected allowed_statuses to be []map[string]any")
	}
	if len(allowedStatuses) != 2 {
		t.Fatalf("expected 2 allowed statuses, got %d", len(allowedStatuses))
	}

	// Attachments
	attachments, ok := result["attachments"].([]map[string]any)
	if !ok {
		t.Fatal("expected attachments to be []map[string]any")
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0]["filename"] != "spec.pdf" {
		t.Errorf("expected attachment filename='spec.pdf', got %v", attachments[0]["filename"])
	}
	if attachments[0]["filesize"] != 1024 {
		t.Errorf("expected attachment filesize=1024, got %v", attachments[0]["filesize"])
	}
	if attachments[0]["content_type"] != "application/pdf" {
		t.Errorf("expected attachment content_type='application/pdf', got %v", attachments[0]["content_type"])
	}
	authorInAttachment, ok := attachments[0]["author"].(map[string]any)
	if !ok {
		t.Fatal("expected attachment author to be map[string]any")
	}
	if authorInAttachment["name"] != "Carol" {
		t.Errorf("expected attachment author name='Carol', got %v", authorInAttachment["name"])
	}
}

func TestFormatIssueDetail_EmptyOptionalFields(t *testing.T) {
	issue := redmine.Issue{
		ID:      1,
		Project: redmine.IDName{ID: 1, Name: "P"},
		Tracker: redmine.IDName{ID: 1, Name: "T"},
		Status:  redmine.IDName{ID: 1, Name: "S"},
		Priority: redmine.IDName{ID: 1, Name: "Pr"},
		Author:  redmine.IDName{ID: 1, Name: "A"},
		Subject: "Minimal issue",
	}

	result := formatIssueDetail(issue)

	// description and done_ratio should always be present in detail
	if result["description"] != "" {
		t.Errorf("expected empty description, got %v", result["description"])
	}
	if result["done_ratio"] != 0 {
		t.Errorf("expected done_ratio=0, got %v", result["done_ratio"])
	}

	// Optional slices should be absent when empty
	if _, exists := result["parent_issue_id"]; exists {
		t.Error("expected parent_issue_id to be absent when parent is nil")
	}
	if _, exists := result["custom_fields"]; exists {
		t.Error("expected custom_fields to be absent when empty")
	}
	if _, exists := result["journals"]; exists {
		t.Error("expected journals to be absent when empty")
	}
	if _, exists := result["watchers"]; exists {
		t.Error("expected watchers to be absent when empty")
	}
	if _, exists := result["relations"]; exists {
		t.Error("expected relations to be absent when empty")
	}
	if _, exists := result["allowed_statuses"]; exists {
		t.Error("expected allowed_statuses to be absent when empty")
	}
	if _, exists := result["attachments"]; exists {
		t.Error("expected attachments to be absent when empty")
	}
}

// --- TestJsonResult ---

func TestJsonResult(t *testing.T) {
	t.Run("simple map produces valid JSON", func(t *testing.T) {
		data := map[string]any{
			"id":   42,
			"name": "test",
		}

		result, err := jsonResult(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.IsError {
			t.Error("expected IsError=false")
		}
		if len(result.Content) == 0 {
			t.Fatal("expected non-empty content")
		}

		// Extract text from result
		textContent, ok := result.Content[0].(gomcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}

		// Verify the text is valid JSON
		var parsed map[string]any
		if err := json.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
			t.Fatalf("result text is not valid JSON: %v", err)
		}

		// Verify JSON content matches input
		id, ok := parsed["id"].(float64) // JSON numbers are float64
		if !ok || int(id) != 42 {
			t.Errorf("expected id=42, got %v", parsed["id"])
		}
		if parsed["name"] != "test" {
			t.Errorf("expected name='test', got %v", parsed["name"])
		}
	})

	t.Run("nested structure", func(t *testing.T) {
		data := map[string]any{
			"issues": []map[string]any{
				{"id": 1, "subject": "Issue 1"},
				{"id": 2, "subject": "Issue 2"},
			},
			"total_count": 2,
		}

		result, err := jsonResult(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		textContent, ok := result.Content[0].(gomcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
			t.Fatalf("result text is not valid JSON: %v", err)
		}

		issues, ok := parsed["issues"].([]any)
		if !ok {
			t.Fatal("expected issues to be an array")
		}
		if len(issues) != 2 {
			t.Errorf("expected 2 issues, got %d", len(issues))
		}
	})

	t.Run("nil data", func(t *testing.T) {
		result, err := jsonResult(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		// nil marshals to "null"
		textContent, ok := result.Content[0].(gomcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}
		if textContent.Text != "null" {
			t.Errorf("expected 'null' for nil data, got %q", textContent.Text)
		}
	})
}

// --- TestGetMapArg ---

func TestGetMapArg(t *testing.T) {
	t.Run("returns map when present", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"custom_fields": map[string]any{
				"Component": "SW Tool",
			},
		}

		result := getMapArg(req, "custom_fields")
		if result == nil {
			t.Fatal("expected non-nil map")
		}
		if result["Component"] != "SW Tool" {
			t.Errorf("expected Component='SW Tool', got %v", result["Component"])
		}
	})

	t.Run("returns nil when key missing", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"other_key": "value",
		}

		result := getMapArg(req, "custom_fields")
		if result != nil {
			t.Errorf("expected nil for missing key, got %v", result)
		}
	})

	t.Run("returns nil when value is not a map", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"custom_fields": "not a map",
		}

		result := getMapArg(req, "custom_fields")
		if result != nil {
			t.Errorf("expected nil for non-map value, got %v", result)
		}
	})

	t.Run("returns nil when arguments is nil", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		// Arguments is nil by default

		result := getMapArg(req, "custom_fields")
		if result != nil {
			t.Errorf("expected nil for nil arguments, got %v", result)
		}
	})
}

// --- TestGetArrayArg ---

func TestGetArrayArg(t *testing.T) {
	t.Run("returns array when present", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"issue_ids": []any{float64(1), float64(2), float64(3)},
		}

		result := getArrayArg(req, "issue_ids")
		if result == nil {
			t.Fatal("expected non-nil array")
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 elements, got %d", len(result))
		}
		if result[0] != float64(1) {
			t.Errorf("expected first element=1, got %v", result[0])
		}
	})

	t.Run("returns nil when key missing", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"other_key": "value",
		}

		result := getArrayArg(req, "issue_ids")
		if result != nil {
			t.Errorf("expected nil for missing key, got %v", result)
		}
	})

	t.Run("returns nil when value is not an array", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"issue_ids": "not an array",
		}

		result := getArrayArg(req, "issue_ids")
		if result != nil {
			t.Errorf("expected nil for non-array value, got %v", result)
		}
	})

	t.Run("returns empty array when empty", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"issue_ids": []any{},
		}

		result := getArrayArg(req, "issue_ids")
		if result == nil {
			t.Fatal("expected non-nil array for empty slice")
		}
		if len(result) != 0 {
			t.Errorf("expected 0 elements, got %d", len(result))
		}
	})

	t.Run("returns nil when arguments is nil", func(t *testing.T) {
		req := gomcp.CallToolRequest{}

		result := getArrayArg(req, "issue_ids")
		if result != nil {
			t.Errorf("expected nil for nil arguments, got %v", result)
		}
	})
}

// --- TestParseUploadTokens ---

func TestParseUploadTokens(t *testing.T) {
	t.Run("valid tokens with all fields", func(t *testing.T) {
		tokens := []any{
			map[string]any{
				"token":        "abc123",
				"filename":     "report.pdf",
				"content_type": "application/pdf",
				"description":  "Monthly report",
			},
		}

		result, err := parseUploadTokens(tokens)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 token, got %d", len(result))
		}
		if result[0].Token != "abc123" {
			t.Errorf("expected token='abc123', got %v", result[0].Token)
		}
		if result[0].Filename != "report.pdf" {
			t.Errorf("expected filename='report.pdf', got %v", result[0].Filename)
		}
		if result[0].ContentType != "application/pdf" {
			t.Errorf("expected content_type='application/pdf', got %v", result[0].ContentType)
		}
		if result[0].Description != "Monthly report" {
			t.Errorf("expected description='Monthly report', got %v", result[0].Description)
		}
	})

	t.Run("valid tokens with only required fields", func(t *testing.T) {
		tokens := []any{
			map[string]any{
				"token":    "xyz789",
				"filename": "data.csv",
			},
		}

		result, err := parseUploadTokens(tokens)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 token, got %d", len(result))
		}
		if result[0].Token != "xyz789" {
			t.Errorf("expected token='xyz789', got %v", result[0].Token)
		}
		if result[0].Filename != "data.csv" {
			t.Errorf("expected filename='data.csv', got %v", result[0].Filename)
		}
		if result[0].ContentType != "" {
			t.Errorf("expected empty content_type, got %v", result[0].ContentType)
		}
		if result[0].Description != "" {
			t.Errorf("expected empty description, got %v", result[0].Description)
		}
	})

	t.Run("multiple tokens", func(t *testing.T) {
		tokens := []any{
			map[string]any{"token": "t1", "filename": "f1.txt"},
			map[string]any{"token": "t2", "filename": "f2.txt"},
			map[string]any{"token": "t3", "filename": "f3.txt"},
		}

		result, err := parseUploadTokens(tokens)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 tokens, got %d", len(result))
		}
	})

	t.Run("missing token field returns error", func(t *testing.T) {
		tokens := []any{
			map[string]any{
				"filename": "report.pdf",
			},
		}

		_, err := parseUploadTokens(tokens)
		if err == nil {
			t.Fatal("expected error for missing token field")
		}
	})

	t.Run("missing filename field returns error", func(t *testing.T) {
		tokens := []any{
			map[string]any{
				"token": "abc123",
			},
		}

		_, err := parseUploadTokens(tokens)
		if err == nil {
			t.Fatal("expected error for missing filename field")
		}
	})

	t.Run("non-object item returns error", func(t *testing.T) {
		tokens := []any{
			"not an object",
		}

		_, err := parseUploadTokens(tokens)
		if err == nil {
			t.Fatal("expected error for non-object item")
		}
	})

	t.Run("empty array returns empty slice", func(t *testing.T) {
		tokens := []any{}

		result, err := parseUploadTokens(tokens)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected 0 tokens, got %d", len(result))
		}
	})

	t.Run("empty token string returns error", func(t *testing.T) {
		tokens := []any{
			map[string]any{
				"token":    "",
				"filename": "report.pdf",
			},
		}

		_, err := parseUploadTokens(tokens)
		if err == nil {
			t.Fatal("expected error for empty token string")
		}
	})

	t.Run("empty filename string returns error", func(t *testing.T) {
		tokens := []any{
			map[string]any{
				"token":    "abc123",
				"filename": "",
			},
		}

		_, err := parseUploadTokens(tokens)
		if err == nil {
			t.Fatal("expected error for empty filename string")
		}
	})
}

// --- TestHandleSearchGlobal ---

func TestHandleSearchGlobal(t *testing.T) {
	// Set up a mock Redmine server that responds to /search.json
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing query", http.StatusBadRequest)
			return
		}

		resp := map[string]any{
			"results": []map[string]any{
				{
					"id":          42,
					"title":       "Test Issue #42",
					"type":        "issue",
					"url":         "/issues/42",
					"description": "Found by search",
					"datetime":    "2025-01-15T10:30:00Z",
				},
				{
					"id":          7,
					"title":       "Wiki: Getting Started",
					"type":        "wiki-page",
					"url":         "/projects/demo/wiki/Getting_Started",
					"description": "",
					"datetime":    "2025-01-10T08:00:00Z",
				},
			},
			"total_count": 2,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	client := redmine.NewClient(mockServer.URL, "test-api-key")
	h := NewToolHandlers(client, nil, nil)

	t.Run("basic search returns results", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"q": "test",
		}

		result, err := h.handleSearchGlobal(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.IsError {
			t.Fatalf("expected success, got error: %v", result.Content)
		}

		// Parse the JSON text result
		textContent, ok := result.Content[0].(gomcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
			t.Fatalf("failed to parse result JSON: %v", err)
		}

		if data["count"].(float64) != 2 {
			t.Errorf("expected count=2, got %v", data["count"])
		}
		if data["total_count"].(float64) != 2 {
			t.Errorf("expected total_count=2, got %v", data["total_count"])
		}

		results, ok := data["results"].([]any)
		if !ok || len(results) != 2 {
			t.Fatalf("expected 2 results, got %v", data["results"])
		}

		first := results[0].(map[string]any)
		if first["id"].(float64) != 42 {
			t.Errorf("expected first result id=42, got %v", first["id"])
		}
		if first["type"].(string) != "issue" {
			t.Errorf("expected first result type='issue', got %v", first["type"])
		}
		if first["title"].(string) != "Test Issue #42" {
			t.Errorf("expected first result title='Test Issue #42', got %v", first["title"])
		}
	})

	t.Run("missing query returns error", func(t *testing.T) {
		req := gomcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{}

		result, err := h.handleSearchGlobal(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing query")
		}
	})
}

func TestResolveCustomFieldsRequiredValidation(t *testing.T) {
	// Mock Redmine server that returns empty issues (no custom field defs from API)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issues":      []any{},
			"total_count": 0,
		})
	}))
	defer mockServer.Close()

	rules := &redmine.CustomFieldRules{
		Fields: map[string]redmine.CustomFieldRule{
			"223": {
				Name:               "SW_Category",
				Values:             []string{"Debug", "Other"},
				RequiredByTrackers: []int{32}, // SW_Task
			},
			"27": {
				Name:               "HW Version",
				RequiredByTrackers: []int{4, 22}, // Bug, EE_Task
			},
			"23": {
				Name:   "Component",
				Values: []string{"HW", "SW"},
			},
		},
	}

	client := redmine.NewClient(mockServer.URL, "test-api-key")
	h := NewToolHandlers(client, rules, nil)

	t.Run("missing required field for matching tracker returns error", func(t *testing.T) {
		fields := map[string]any{}
		_, err := h.resolveCustomFields(fields, 1, 32) // tracker 32 = SW_Task
		if err == nil {
			t.Fatal("expected error for missing required field")
		}
		if !strings.Contains(err.Error(), "required custom field(s) missing") {
			t.Fatalf("expected 'required custom field(s) missing' in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "SW_Category") {
			t.Fatalf("expected 'SW_Category' in error, got: %v", err)
		}
	})

	t.Run("field required for tracker A not required for tracker B", func(t *testing.T) {
		fields := map[string]any{}
		// tracker 1 = Requirement (no required fields)
		result, err := h.resolveCustomFields(fields, 1, 1)
		if err != nil {
			t.Fatalf("unexpected error for tracker with no required fields: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty result, got %v", result)
		}
	})

	t.Run("providing required field passes validation", func(t *testing.T) {
		fields := map[string]any{
			"223": "Debug",
		}
		result, err := h.resolveCustomFields(fields, 1, 32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["223"] != "Debug" {
			t.Fatalf("expected 'Debug', got %v", result["223"])
		}
	})

	t.Run("providing required field by name passes validation", func(t *testing.T) {
		fields := map[string]any{
			"SW_Category": "Other",
		}
		result, err := h.resolveCustomFields(fields, 1, 32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["223"] != "Other" {
			t.Fatalf("expected 'Other', got %v", result["223"])
		}
	})

	t.Run("multiple required fields for Bug tracker", func(t *testing.T) {
		fields := map[string]any{}
		_, err := h.resolveCustomFields(fields, 1, 4) // tracker 4 = Bug
		if err == nil {
			t.Fatal("expected error for missing required fields")
		}
		if !strings.Contains(err.Error(), "HW Version") {
			t.Fatalf("expected 'HW Version' in error, got: %v", err)
		}
	})

	t.Run("trackerID 0 skips required check", func(t *testing.T) {
		fields := map[string]any{}
		result, err := h.resolveCustomFields(fields, 1, 0)
		if err != nil {
			t.Fatalf("unexpected error with trackerID 0: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty result, got %v", result)
		}
	})

	t.Run("no required fields with nil rules passes", func(t *testing.T) {
		hNoRules := NewToolHandlers(client, nil, nil)
		fields := map[string]any{}
		result, err := hNoRules.resolveCustomFields(fields, 1, 32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty result, got %v", result)
		}
	})
}
