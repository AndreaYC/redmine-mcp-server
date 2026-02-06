package redmine

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// CustomFieldRule defines valid values for a custom field
type CustomFieldRule struct {
	Name               string   `json:"name"`
	Values             []string `json:"values"`
	RequiredByTrackers []int    `json:"required_by_trackers,omitempty"`
}

// CustomFieldRules holds validation rules for custom fields
type CustomFieldRules struct {
	Fields map[string]CustomFieldRule `json:"fields"` // field ID (string) → rule
}

// LoadCustomFieldRules loads rules from a JSON file.
// Returns nil (no rules) if the file doesn't exist.
func LoadCustomFieldRules(path string) (*CustomFieldRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read custom field rules: %w", err)
	}

	var rules CustomFieldRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse custom field rules: %w", err)
	}

	return &rules, nil
}

// ValidateValue checks a value against the rules for a given field ID.
// Returns the correctly-cased value if matched, or an error with valid options.
// If no rules exist for the field, the value passes through unchanged.
func (r *CustomFieldRules) ValidateValue(fieldID int, value string) (string, error) {
	if r == nil {
		return value, nil
	}

	rule, ok := r.Fields[strconv.Itoa(fieldID)]
	if !ok {
		return value, nil // no rules for this field, pass through
	}

	// Free-text field (no constrained values) — pass through
	if len(rule.Values) == 0 {
		return value, nil
	}

	// Exact match
	for _, v := range rule.Values {
		if v == value {
			return value, nil
		}
	}

	// Case-insensitive match
	lower := strings.ToLower(value)
	for _, v := range rule.Values {
		if strings.ToLower(v) == lower {
			return v, nil // auto-correct case
		}
	}

	// No match — return error with valid values
	return "", fmt.Errorf("invalid value %q for %s (ID: %d). Valid values: %s",
		value, rule.Name, fieldID, strings.Join(rule.Values, ", "))
}

// ValidateValues checks each value in a slice (for multi-select fields).
func (r *CustomFieldRules) ValidateValues(fieldID int, values []string) ([]string, error) {
	if r == nil {
		return values, nil
	}

	result := make([]string, len(values))
	for i, v := range values {
		corrected, err := r.ValidateValue(fieldID, v)
		if err != nil {
			return nil, err
		}
		result[i] = corrected
	}
	return result, nil
}

// GetRequiredFieldsForTracker returns field IDs and names required for a specific tracker.
func (r *CustomFieldRules) GetRequiredFieldsForTracker(trackerID int) map[int]string {
	result := make(map[int]string)
	if r == nil || trackerID == 0 {
		return result
	}
	for idStr, rule := range r.Fields {
		if !slices.Contains(rule.RequiredByTrackers, trackerID) {
			continue
		}
		if id, err := strconv.Atoi(idStr); err == nil {
			result[id] = rule.Name
		}
	}
	return result
}
