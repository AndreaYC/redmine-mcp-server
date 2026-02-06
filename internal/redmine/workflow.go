package redmine

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"sort"
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

// TrackerTransitionData holds collected transition data for a single tracker.
type TrackerTransitionData struct {
	Name        string
	Transitions map[int][]IDName // fromStatusID → allowed target statuses
}

// GenerateWorkflowOptions configures the workflow generation process.
type GenerateWorkflowOptions struct {
	PerTracker int // max issues to inspect per tracker (default 50)
}

// extractTransitions scans journals for status_id changes and returns
// [from, to] pairs as integer IDs.
func extractTransitions(journals []Journal) [][2]int {
	var pairs [][2]int
	for _, j := range journals {
		for _, d := range j.Details {
			if d.Property != "attr" || d.Name != "status_id" {
				continue
			}
			oldID, err1 := strconv.Atoi(d.OldValue)
			newID, err2 := strconv.Atoi(d.NewValue)
			if err1 != nil || err2 != nil {
				continue
			}
			pairs = append(pairs, [2]int{oldID, newID})
		}
	}
	return pairs
}

// BuildWorkflowRules converts collected data into WorkflowRules.
// statuses: global status list (for IsClosed lookup)
// trackerTransitions: map[trackerID] → TrackerTransitionData
func BuildWorkflowRules(statuses []IssueStatus, trackerTransitions map[int]TrackerTransitionData) *WorkflowRules {
	// Build status lookup for IsClosed
	statusLookup := make(map[int]IssueStatus, len(statuses))
	for _, s := range statuses {
		statusLookup[s.ID] = s
	}

	rules := &WorkflowRules{Trackers: make(map[string]WorkflowTracker)}

	for trackerID, data := range trackerTransitions {
		if len(data.Transitions) == 0 {
			continue
		}

		// Collect all referenced status IDs
		referenced := make(map[int]struct{})
		for fromID, targets := range data.Transitions {
			referenced[fromID] = struct{}{}
			for _, t := range targets {
				referenced[t.ID] = struct{}{}
			}
		}

		// Build statuses map with only referenced statuses
		wfStatuses := make(map[string]WorkflowStatus, len(referenced))
		for sid := range referenced {
			gs, ok := statusLookup[sid]
			if !ok {
				continue
			}
			wfStatuses[strconv.Itoa(sid)] = WorkflowStatus{
				Name:     gs.Name,
				IsClosed: gs.IsClosed,
			}
		}

		// Build transitions map (deduplicate targets)
		transitions := make(map[string][]int, len(data.Transitions))
		for fromID, targets := range data.Transitions {
			seen := make(map[int]struct{})
			var ids []int
			for _, t := range targets {
				if _, exists := seen[t.ID]; !exists {
					seen[t.ID] = struct{}{}
					ids = append(ids, t.ID)
				}
			}
			sort.Ints(ids)
			transitions[strconv.Itoa(fromID)] = ids
		}

		rules.Trackers[strconv.Itoa(trackerID)] = WorkflowTracker{
			Name:        data.Name,
			Statuses:    wfStatuses,
			Transitions: transitions,
		}
	}

	return rules
}

// GenerateWorkflowRules crawls existing issues to infer workflow transition rules
// from journal history (status_id changes). This works on all Redmine versions,
// including those that don't support the allowed_statuses field.
func GenerateWorkflowRules(client *Client, opts GenerateWorkflowOptions, logf func(string, ...any)) (*WorkflowRules, error) {
	if opts.PerTracker <= 0 {
		opts.PerTracker = 50
	}

	trackers, err := client.ListTrackers()
	if err != nil {
		return nil, fmt.Errorf("failed to list trackers: %w", err)
	}

	statuses, err := client.ListIssueStatuses()
	if err != nil {
		return nil, fmt.Errorf("failed to list issue statuses: %w", err)
	}

	// Build status name lookup
	statusLookup := make(map[int]string, len(statuses))
	for _, s := range statuses {
		statusLookup[s.ID] = s.Name
	}

	trackerTransitions := make(map[int]TrackerTransitionData)

	for i, tracker := range trackers {
		logf("tracker %d/%d: %s", i+1, len(trackers), tracker.Name)

		// Fetch issues for this tracker, paginated, sorted by most recently updated
		var allIssues []Issue
		remaining := opts.PerTracker
		offset := 0
		for remaining > 0 {
			limit := min(remaining, 100)
			issues, _, err := client.SearchIssues(SearchIssuesParams{
				TrackerID: tracker.ID,
				StatusID:  "*",
				Sort:      "updated_on:desc",
				Limit:     limit,
				Offset:    offset,
			})
			if err != nil {
				logf("  warning: failed to search issues for tracker %s: %v", tracker.Name, err)
				break
			}
			allIssues = append(allIssues, issues...)
			if len(issues) < limit {
				break // no more pages
			}
			remaining -= len(issues)
			offset += len(issues)
		}

		if len(allIssues) == 0 {
			logf("  no issues found, skipping")
			continue
		}

		data := TrackerTransitionData{
			Name:        tracker.Name,
			Transitions: make(map[int][]IDName),
		}

		transitionCount := 0
		for _, brief := range allIssues {
			issue, err := client.GetIssue(brief.ID)
			if err != nil {
				logf("  warning: failed to get issue #%d: %v", brief.ID, err)
				continue
			}
			for _, pair := range extractTransitions(issue.Journals) {
				fromID, toID := pair[0], pair[1]
				toName := statusLookup[toID]
				if toName == "" {
					toName = fmt.Sprintf("Unknown(%d)", toID)
				}
				data.Transitions[fromID] = append(data.Transitions[fromID], IDName{ID: toID, Name: toName})
				transitionCount++
			}
		}

		logf("  %d issues inspected, %d transitions found", len(allIssues), transitionCount)
		trackerTransitions[tracker.ID] = data
	}

	return BuildWorkflowRules(statuses, trackerTransitions), nil
}

// Merge updates r with trackers from other. Trackers present in other overwrite
// those in r; trackers only in r are preserved.
func (r *WorkflowRules) Merge(other *WorkflowRules) {
	if other == nil {
		return
	}
	maps.Copy(r.Trackers, other.Trackers)
}
