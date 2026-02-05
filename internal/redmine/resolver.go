package redmine

import (
	"fmt"
	"strconv"
	"strings"
)

// Resolver helps resolve names to IDs
type Resolver struct {
	client *Client

	// Cached data
	trackers     []Tracker
	statuses     []IssueStatus
	priorities   []IssuePriority
	projects     []Project
	activities   []TimeEntryActivity
	customFields []CustomFieldDefinitionFull
}

// NewResolver creates a new resolver
func NewResolver(client *Client) *Resolver {
	return &Resolver{client: client}
}

// ResolveError represents an error when resolving a name
type ResolveError struct {
	Type     string
	Query    string
	Matches  []IDName
	NotFound bool
}

func (e *ResolveError) Error() string {
	if e.NotFound {
		return fmt.Sprintf("%s not found: %s", e.Type, e.Query)
	}
	names := make([]string, len(e.Matches))
	for i, m := range e.Matches {
		names[i] = fmt.Sprintf("%s (ID: %d)", m.Name, m.ID)
	}
	return fmt.Sprintf("multiple %s match '%s': %s", e.Type, e.Query, strings.Join(names, ", "))
}

// ResolveProject resolves a project name or ID to a project ID
func (r *Resolver) ResolveProject(nameOrID string) (int, error) {
	// Try parsing as ID first
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	// Load projects if not cached
	if r.projects == nil {
		projects, err := r.client.ListProjects(1000)
		if err != nil {
			return 0, fmt.Errorf("failed to load projects: %w", err)
		}
		r.projects = projects
	}

	// Search by name or identifier (case-insensitive)
	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, p := range r.projects {
		if strings.ToLower(p.Name) == query || strings.ToLower(p.Identifier) == query {
			matches = append(matches, IDName{ID: p.ID, Name: p.Name})
		}
	}

	// If no exact match, try partial match
	if len(matches) == 0 {
		for _, p := range r.projects {
			if strings.Contains(strings.ToLower(p.Name), query) {
				matches = append(matches, IDName{ID: p.ID, Name: p.Name})
			}
		}
	}

	if len(matches) == 0 {
		return 0, &ResolveError{Type: "project", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return 0, &ResolveError{Type: "project", Query: nameOrID, Matches: matches}
	}

	return matches[0].ID, nil
}

// ResolveTracker resolves a tracker name or ID to a tracker ID
func (r *Resolver) ResolveTracker(nameOrID string) (int, error) {
	// Try parsing as ID first
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	// Load trackers if not cached
	if r.trackers == nil {
		trackers, err := r.client.ListTrackers()
		if err != nil {
			return 0, fmt.Errorf("failed to load trackers: %w", err)
		}
		r.trackers = trackers
	}

	// Search by name (case-insensitive)
	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, t := range r.trackers {
		if strings.ToLower(t.Name) == query {
			matches = append(matches, IDName(t))
		}
	}

	// If no exact match, try partial match
	if len(matches) == 0 {
		for _, t := range r.trackers {
			if strings.Contains(strings.ToLower(t.Name), query) {
				matches = append(matches, IDName(t))
			}
		}
	}

	if len(matches) == 0 {
		return 0, &ResolveError{Type: "tracker", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return 0, &ResolveError{Type: "tracker", Query: nameOrID, Matches: matches}
	}

	return matches[0].ID, nil
}

// ResolveStatus resolves a status name or ID to a status ID
// Special values: "open", "closed", "*" (all)
func (r *Resolver) ResolveStatus(nameOrID string) (string, error) {
	// Handle special values
	switch strings.ToLower(nameOrID) {
	case "open", "closed", "*", "all":
		if nameOrID == "all" {
			return "*", nil
		}
		return strings.ToLower(nameOrID), nil
	}

	// Try parsing as ID first
	if _, err := strconv.Atoi(nameOrID); err == nil {
		return nameOrID, nil
	}

	// Load statuses if not cached
	if r.statuses == nil {
		statuses, err := r.client.ListIssueStatuses()
		if err != nil {
			return "", fmt.Errorf("failed to load statuses: %w", err)
		}
		r.statuses = statuses
	}

	// Search by name (case-insensitive)
	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, s := range r.statuses {
		if strings.ToLower(s.Name) == query {
			matches = append(matches, IDName{ID: s.ID, Name: s.Name})
		}
	}

	// If no exact match, try partial match
	if len(matches) == 0 {
		for _, s := range r.statuses {
			if strings.Contains(strings.ToLower(s.Name), query) {
				matches = append(matches, IDName{ID: s.ID, Name: s.Name})
			}
		}
	}

	if len(matches) == 0 {
		return "", &ResolveError{Type: "status", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return "", &ResolveError{Type: "status", Query: nameOrID, Matches: matches}
	}

	return strconv.Itoa(matches[0].ID), nil
}

// ResolveStatusID resolves a status name or ID to a status ID (int)
func (r *Resolver) ResolveStatusID(nameOrID string) (int, error) {
	// Try parsing as ID first
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	// Load statuses if not cached
	if r.statuses == nil {
		statuses, err := r.client.ListIssueStatuses()
		if err != nil {
			return 0, fmt.Errorf("failed to load statuses: %w", err)
		}
		r.statuses = statuses
	}

	// Search by name (case-insensitive)
	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, s := range r.statuses {
		if strings.ToLower(s.Name) == query {
			matches = append(matches, IDName{ID: s.ID, Name: s.Name})
		}
	}

	if len(matches) == 0 {
		for _, s := range r.statuses {
			if strings.Contains(strings.ToLower(s.Name), query) {
				matches = append(matches, IDName{ID: s.ID, Name: s.Name})
			}
		}
	}

	if len(matches) == 0 {
		return 0, &ResolveError{Type: "status", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return 0, &ResolveError{Type: "status", Query: nameOrID, Matches: matches}
	}

	return matches[0].ID, nil
}

// ResolvePriority resolves a priority name or ID to a priority ID
func (r *Resolver) ResolvePriority(nameOrID string) (int, error) {
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	if r.priorities == nil {
		priorities, err := r.client.ListIssuePriorities()
		if err != nil {
			return 0, fmt.Errorf("failed to load priorities: %w", err)
		}
		r.priorities = priorities
	}

	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, p := range r.priorities {
		if strings.ToLower(p.Name) == query {
			matches = append(matches, IDName{ID: p.ID, Name: p.Name})
		}
	}

	if len(matches) == 0 {
		for _, p := range r.priorities {
			if strings.Contains(strings.ToLower(p.Name), query) {
				matches = append(matches, IDName{ID: p.ID, Name: p.Name})
			}
		}
	}

	if len(matches) == 0 {
		return 0, &ResolveError{Type: "priority", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return 0, &ResolveError{Type: "priority", Query: nameOrID, Matches: matches}
	}

	return matches[0].ID, nil
}

// ResolveActivity resolves an activity name or ID to an activity ID
func (r *Resolver) ResolveActivity(nameOrID string) (int, error) {
	// Try parsing as ID first
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	// Load activities if not cached
	if r.activities == nil {
		activities, err := r.client.ListTimeEntryActivities()
		if err != nil {
			return 0, fmt.Errorf("failed to load activities: %w", err)
		}
		r.activities = activities
	}

	// Search by name (case-insensitive)
	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, a := range r.activities {
		if strings.ToLower(a.Name) == query {
			matches = append(matches, IDName{ID: a.ID, Name: a.Name})
		}
	}

	if len(matches) == 0 {
		for _, a := range r.activities {
			if strings.Contains(strings.ToLower(a.Name), query) {
				matches = append(matches, IDName{ID: a.ID, Name: a.Name})
			}
		}
	}

	if len(matches) == 0 {
		return 0, &ResolveError{Type: "activity", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return 0, &ResolveError{Type: "activity", Query: nameOrID, Matches: matches}
	}

	return matches[0].ID, nil
}

// ResolveUser resolves a user name, email, or ID to a user ID
// Uses project memberships since /users.json requires admin
func (r *Resolver) ResolveUser(nameOrID string, projectID int) (int, error) {
	// Handle special value
	if strings.ToLower(nameOrID) == "me" {
		user, err := r.client.GetCurrentUser()
		if err != nil {
			return 0, err
		}
		return user.ID, nil
	}

	// Try parsing as ID first
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	// If no project ID, we can't search users
	if projectID <= 0 {
		return 0, fmt.Errorf("cannot search users without project context, please use user ID")
	}

	// Get project memberships
	memberships, err := r.client.GetProjectMemberships(projectID, 1000)
	if err != nil {
		return 0, fmt.Errorf("failed to load project memberships: %w", err)
	}

	// Search by name or email (case-insensitive)
	query := strings.ToLower(nameOrID)
	var matches []IDName
	for _, m := range memberships {
		if m.User == nil {
			continue
		}
		name := strings.ToLower(m.User.Name)
		if name == query || strings.Contains(name, query) {
			// Check for duplicates
			found := false
			for _, match := range matches {
				if match.ID == m.User.ID {
					found = true
					break
				}
			}
			if !found {
				matches = append(matches, *m.User)
			}
		}
	}

	if len(matches) == 0 {
		return 0, &ResolveError{Type: "user", Query: nameOrID, NotFound: true}
	}
	if len(matches) > 1 {
		return 0, &ResolveError{Type: "user", Query: nameOrID, Matches: matches}
	}

	return matches[0].ID, nil
}

// ResolveCustomFieldID resolves a custom field name to its ID
// This requires fetching an issue to get the custom field mapping
func (r *Resolver) ResolveCustomFieldID(name string, sampleIssueID int) (int, error) {
	// Try parsing as ID first
	if id, err := strconv.Atoi(name); err == nil {
		return id, nil
	}

	// Fetch sample issue to get custom field mapping
	issue, err := r.client.GetIssue(sampleIssueID)
	if err != nil {
		return 0, fmt.Errorf("failed to get sample issue for custom field mapping: %w", err)
	}

	query := strings.ToLower(name)
	for _, cf := range issue.CustomFields {
		if strings.ToLower(cf.Name) == query {
			return cf.ID, nil
		}
	}

	return 0, &ResolveError{Type: "custom field", Query: name, NotFound: true}
}

// GetTrackers returns all trackers
func (r *Resolver) GetTrackers() ([]Tracker, error) {
	if r.trackers == nil {
		trackers, err := r.client.ListTrackers()
		if err != nil {
			return nil, err
		}
		r.trackers = trackers
	}
	return r.trackers, nil
}

// GetStatuses returns all statuses
func (r *Resolver) GetStatuses() ([]IssueStatus, error) {
	if r.statuses == nil {
		statuses, err := r.client.ListIssueStatuses()
		if err != nil {
			return nil, err
		}
		r.statuses = statuses
	}
	return r.statuses, nil
}

// GetActivities returns all time entry activities
func (r *Resolver) GetActivities() ([]TimeEntryActivity, error) {
	if r.activities == nil {
		activities, err := r.client.ListTimeEntryActivities()
		if err != nil {
			return nil, err
		}
		r.activities = activities
	}
	return r.activities, nil
}

// ResolveCustomFieldByName resolves a custom field name or ID to its ID.
// First tries admin API cache, falls back to project-specific fields if admin API fails.
func (r *Resolver) ResolveCustomFieldByName(nameOrID string, projectID int, trackerID int) (int, error) {
	// Try parsing as ID first
	if id, err := strconv.Atoi(nameOrID); err == nil {
		return id, nil
	}

	// Try admin API cache first
	if r.customFields == nil {
		fields, err := r.client.ListAllCustomFields()
		if err == nil {
			r.customFields = fields
		}
		// If admin API fails, customFields stays nil — we'll fall back below
	}

	query := strings.ToLower(nameOrID)

	// Search in admin API cache if available
	if r.customFields != nil {
		var matches []IDName
		for _, cf := range r.customFields {
			if strings.ToLower(cf.Name) == query {
				matches = append(matches, IDName{ID: cf.ID, Name: cf.Name})
			}
		}
		if len(matches) == 0 {
			for _, cf := range r.customFields {
				if strings.Contains(strings.ToLower(cf.Name), query) {
					matches = append(matches, IDName{ID: cf.ID, Name: cf.Name})
				}
			}
		}
		if len(matches) == 1 {
			return matches[0].ID, nil
		}
		if len(matches) > 1 {
			return 0, &ResolveError{Type: "custom field", Query: nameOrID, Matches: matches}
		}
		// Not found in admin cache — return not found
		return 0, &ResolveError{Type: "custom field", Query: nameOrID, NotFound: true}
	}

	// Fallback: use project-specific custom fields (from issue inspection)
	if projectID > 0 {
		defs, err := r.client.GetProjectCustomFields(projectID, trackerID)
		if err == nil {
			for _, def := range defs {
				if strings.ToLower(def.Name) == query {
					return def.ID, nil
				}
			}
		}
	}

	return 0, &ResolveError{Type: "custom field", Query: nameOrID, NotFound: true}
}

// GetPriorities returns all issue priorities
func (r *Resolver) GetPriorities() ([]IssuePriority, error) {
	if r.priorities == nil {
		priorities, err := r.client.ListIssuePriorities()
		if err != nil {
			return nil, err
		}
		r.priorities = priorities
	}
	return r.priorities, nil
}

// GetCustomFields returns all custom field definitions (requires admin)
func (r *Resolver) GetCustomFields() ([]CustomFieldDefinitionFull, error) {
	if r.customFields == nil {
		fields, err := r.client.ListAllCustomFields()
		if err != nil {
			return nil, err
		}
		r.customFields = fields
	}
	return r.customFields, nil
}
