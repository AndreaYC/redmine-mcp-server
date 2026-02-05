package redmine

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// WorkflowStatus defines a status within a tracker workflow
type WorkflowStatus struct {
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
}

// WorkflowTracker defines the workflow for a single tracker
type WorkflowTracker struct {
	Name        string                    `json:"name"`
	Statuses    map[string]WorkflowStatus `json:"statuses"`
	Transitions map[string][]int          `json:"transitions"`
}

// WorkflowRules holds workflow transition rules for all trackers
type WorkflowRules struct {
	Trackers map[string]WorkflowTracker `json:"trackers"`
}

// LoadWorkflowRules loads workflow rules from a JSON file.
// Returns nil (no rules) if the file doesn't exist.
func LoadWorkflowRules(path string) (*WorkflowRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workflow rules: %w", err)
	}

	var rules WorkflowRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse workflow rules: %w", err)
	}

	return &rules, nil
}

// ValidateTransition checks whether a status transition is allowed.
// Returns nil if allowed, or an error with allowed targets listed.
// Passes through (returns nil) if rules are nil, tracker unknown, or from_status unknown.
func (w *WorkflowRules) ValidateTransition(trackerID, fromStatusID, toStatusID int) error {
	if w == nil {
		return nil
	}

	tracker, ok := w.Trackers[strconv.Itoa(trackerID)]
	if !ok {
		return nil // unknown tracker, pass through
	}

	fromKey := strconv.Itoa(fromStatusID)
	allowed, ok := tracker.Transitions[fromKey]
	if !ok {
		return nil // unknown from_status, pass through
	}

	for _, id := range allowed {
		if id == toStatusID {
			return nil // transition is allowed
		}
	}

	// Build helpful error message
	fromName := statusName(tracker, fromStatusID)
	toName := statusName(tracker, toStatusID)

	allowedNames := make([]string, len(allowed))
	for i, id := range allowed {
		allowedNames[i] = statusName(tracker, id)
	}

	if len(allowed) == 0 {
		return fmt.Errorf("cannot change %s from '%s' to '%s': no transitions allowed from '%s'",
			tracker.Name, fromName, toName, fromName)
	}

	return fmt.Errorf("cannot change %s from '%s' to '%s'. Allowed: %s",
		tracker.Name, fromName, toName, strings.Join(allowedNames, ", "))
}

// GetAllowedStatuses returns the list of allowed target statuses for a given tracker and current status.
// Returns nil if rules are nil, tracker unknown, or status unknown.
func (w *WorkflowRules) GetAllowedStatuses(trackerID, fromStatusID int) []IDName {
	if w == nil {
		return nil
	}

	tracker, ok := w.Trackers[strconv.Itoa(trackerID)]
	if !ok {
		return nil
	}

	fromKey := strconv.Itoa(fromStatusID)
	allowed, ok := tracker.Transitions[fromKey]
	if !ok {
		return nil
	}

	result := make([]IDName, len(allowed))
	for i, id := range allowed {
		result[i] = IDName{
			ID:   id,
			Name: statusName(tracker, id),
		}
	}

	return result
}

// GetTrackerStatuses returns all enabled statuses for a tracker.
// Returns nil if rules are nil or tracker unknown.
func (w *WorkflowRules) GetTrackerStatuses(trackerID int) []IDName {
	if w == nil {
		return nil
	}

	tracker, ok := w.Trackers[strconv.Itoa(trackerID)]
	if !ok {
		return nil
	}

	result := make([]IDName, 0, len(tracker.Statuses))
	for idStr, status := range tracker.Statuses {
		id, _ := strconv.Atoi(idStr)
		result = append(result, IDName{ID: id, Name: status.Name})
	}

	return result
}

func statusName(tracker WorkflowTracker, statusID int) string {
	if s, ok := tracker.Statuses[strconv.Itoa(statusID)]; ok {
		return s.Name
	}
	return fmt.Sprintf("Unknown(%d)", statusID)
}
