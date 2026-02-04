package redmine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a Redmine API client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Redmine client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an HTTP request to the Redmine API
func (c *Client) doRequest(method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Redmine-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// User represents a Redmine user
type User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Mail      string `json:"mail"`
	Name      string `json:"name,omitempty"`
}

// CurrentUserResponse is the response from /users/current.json
type CurrentUserResponse struct {
	User User `json:"user"`
}

// GetCurrentUser returns the current user
func (c *Client) GetCurrentUser() (*User, error) {
	data, err := c.doRequest("GET", "/users/current.json", nil)
	if err != nil {
		return nil, err
	}

	var resp CurrentUserResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.User.Name == "" {
		resp.User.Name = resp.User.Firstname + " " + resp.User.Lastname
	}

	return &resp.User, nil
}

// Project represents a Redmine project
type Project struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	Parent      *struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"parent,omitempty"`
}

// ProjectsResponse is the response from /projects.json
type ProjectsResponse struct {
	Projects   []Project `json:"projects"`
	TotalCount int       `json:"total_count"`
	Offset     int       `json:"offset"`
	Limit      int       `json:"limit"`
}

// ListProjects returns all projects
func (c *Client) ListProjects(limit int) ([]Project, error) {
	if limit <= 0 {
		limit = 100
	}

	path := fmt.Sprintf("/projects.json?limit=%d", limit)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp ProjectsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Projects, nil
}

// CreateProjectRequest is the request body for creating a project
type CreateProjectRequest struct {
	Project struct {
		Name        string `json:"name"`
		Identifier  string `json:"identifier"`
		Description string `json:"description,omitempty"`
		ParentID    int    `json:"parent_id,omitempty"`
	} `json:"project"`
}

// CreateProject creates a new project
func (c *Client) CreateProject(name, identifier, description string, parentID int) (*Project, error) {
	req := CreateProjectRequest{}
	req.Project.Name = name
	req.Project.Identifier = identifier
	req.Project.Description = description
	if parentID > 0 {
		req.Project.ParentID = parentID
	}

	data, err := c.doRequest("POST", "/projects.json", req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Project Project `json:"project"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Project, nil
}

// Tracker represents a Redmine tracker
type Tracker struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ListTrackers returns all trackers
func (c *Client) ListTrackers() ([]Tracker, error) {
	data, err := c.doRequest("GET", "/trackers.json", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Trackers []Tracker `json:"trackers"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Trackers, nil
}

// IssueStatus represents a Redmine issue status
type IssueStatus struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
}

// ListIssueStatuses returns all issue statuses
func (c *Client) ListIssueStatuses() ([]IssueStatus, error) {
	data, err := c.doRequest("GET", "/issue_statuses.json", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		IssueStatuses []IssueStatus `json:"issue_statuses"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.IssueStatuses, nil
}

// TimeEntryActivity represents a time entry activity
type TimeEntryActivity struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Active    bool   `json:"active"`
}

// ListTimeEntryActivities returns all time entry activities
func (c *Client) ListTimeEntryActivities() ([]TimeEntryActivity, error) {
	data, err := c.doRequest("GET", "/enumerations/time_entry_activities.json", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		TimeEntryActivities []TimeEntryActivity `json:"time_entry_activities"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.TimeEntryActivities, nil
}

// CustomField represents a custom field value
type CustomField struct {
	ID    int         `json:"id"`
	Name  string      `json:"name"`
	Value any `json:"value"`
}

// CustomFieldDefinition represents a custom field definition (not value)
type CustomFieldDefinition struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	FieldFormat    string   `json:"field_format"`
	Required       bool     `json:"required,omitempty"`
	PossibleValues []string `json:"possible_values,omitempty"`
}

// Issue represents a Redmine issue
type Issue struct {
	ID          int     `json:"id"`
	Project     IDName  `json:"project"`
	Tracker     IDName  `json:"tracker"`
	Status      IDName  `json:"status"`
	Priority    IDName  `json:"priority"`
	Author      IDName  `json:"author"`
	AssignedTo  *IDName `json:"assigned_to,omitempty"`
	Subject     string  `json:"subject"`
	Description string  `json:"description"`
	StartDate   string  `json:"start_date,omitempty"`
	DueDate     string  `json:"due_date,omitempty"`
	DoneRatio   int     `json:"done_ratio"`
	CreatedOn   string  `json:"created_on"`
	UpdatedOn   string  `json:"updated_on"`
	ClosedOn    string  `json:"closed_on,omitempty"`
	Parent      *struct {
		ID int `json:"id"`
	} `json:"parent,omitempty"`
	CustomFields []CustomField `json:"custom_fields,omitempty"`
	Journals     []Journal     `json:"journals,omitempty"`
	Watchers     []IDName      `json:"watchers,omitempty"`
	Relations    []Relation    `json:"relations,omitempty"`
}

// IDName represents a simple id/name pair
type IDName struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Journal represents an issue journal entry (comment/change)
type Journal struct {
	ID        int      `json:"id"`
	User      IDName   `json:"user"`
	Notes     string   `json:"notes"`
	CreatedOn string   `json:"created_on"`
	Details   []Detail `json:"details,omitempty"`
}

// Detail represents a journal detail (field change)
type Detail struct {
	Property string `json:"property"`
	Name     string `json:"name"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

// Relation represents an issue relation
type Relation struct {
	ID           int    `json:"id"`
	IssueID      int    `json:"issue_id"`
	IssueToID    int    `json:"issue_to_id"`
	RelationType string `json:"relation_type"`
	Delay        int    `json:"delay,omitempty"`
}

// IssuesResponse is the response from /issues.json
type IssuesResponse struct {
	Issues     []Issue `json:"issues"`
	TotalCount int     `json:"total_count"`
	Offset     int     `json:"offset"`
	Limit      int     `json:"limit"`
}

// SearchIssuesParams are parameters for searching issues
type SearchIssuesParams struct {
	ProjectID    string
	TrackerID    int
	StatusID     string // "open", "closed", "*", or specific status ID
	AssignedToID string // "me" or user ID
	Subject      string // Search keyword for subject (partial match)
	Limit        int
	Offset       int
}

// SearchIssues searches for issues
func (c *Client) SearchIssues(params SearchIssuesParams) ([]Issue, int, error) {
	query := url.Values{}

	if params.ProjectID != "" {
		query.Set("project_id", params.ProjectID)
	}
	if params.TrackerID > 0 {
		query.Set("tracker_id", strconv.Itoa(params.TrackerID))
	}
	if params.StatusID != "" {
		query.Set("status_id", params.StatusID)
	}
	if params.AssignedToID != "" {
		query.Set("assigned_to_id", params.AssignedToID)
	}
	if params.Subject != "" {
		// Use ~keyword for partial match (contains)
		query.Set("subject", "~"+params.Subject)
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	} else {
		query.Set("limit", "25")
	}
	if params.Offset > 0 {
		query.Set("offset", strconv.Itoa(params.Offset))
	}

	path := "/issues.json?" + query.Encode()
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, 0, err
	}

	var resp IssuesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Issues, resp.TotalCount, nil
}

// GetIssue returns an issue by ID with optional includes
func (c *Client) GetIssue(issueID int) (*Issue, error) {
	path := fmt.Sprintf("/issues/%d.json?include=journals,watchers,relations", issueID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Issue Issue `json:"issue"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Issue, nil
}

// CreateIssueParams are parameters for creating an issue
type CreateIssueParams struct {
	ProjectID     int
	TrackerID     int
	Subject       string
	Description   string
	StatusID      int
	PriorityID    int
	AssignedToID  int
	ParentIssueID int
	StartDate     string
	DueDate       string
	CustomFields  map[string]any
}

// CreateIssue creates a new issue
func (c *Client) CreateIssue(params CreateIssueParams) (*Issue, error) {
	reqBody := map[string]any{
		"issue": map[string]any{
			"project_id": params.ProjectID,
			"tracker_id": params.TrackerID,
			"subject":    params.Subject,
		},
	}

	issueData := reqBody["issue"].(map[string]any)

	if params.Description != "" {
		issueData["description"] = params.Description
	}
	if params.StatusID > 0 {
		issueData["status_id"] = params.StatusID
	}
	if params.PriorityID > 0 {
		issueData["priority_id"] = params.PriorityID
	}
	if params.AssignedToID > 0 {
		issueData["assigned_to_id"] = params.AssignedToID
	}
	if params.ParentIssueID > 0 {
		issueData["parent_issue_id"] = params.ParentIssueID
	}
	if params.StartDate != "" {
		issueData["start_date"] = params.StartDate
	}
	if params.DueDate != "" {
		issueData["due_date"] = params.DueDate
	}

	if len(params.CustomFields) > 0 {
		customFields := make([]map[string]any, 0)
		for id, value := range params.CustomFields {
			cfID, _ := strconv.Atoi(id)
			customFields = append(customFields, map[string]any{
				"id":    cfID,
				"value": value,
			})
		}
		issueData["custom_fields"] = customFields
	}

	data, err := c.doRequest("POST", "/issues.json", reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Issue Issue `json:"issue"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Issue, nil
}

// UpdateIssueParams are parameters for updating an issue
type UpdateIssueParams struct {
	IssueID      int
	StatusID     int
	AssignedToID int
	Notes        string
	CustomFields map[string]any
}

// UpdateIssue updates an existing issue
func (c *Client) UpdateIssue(params UpdateIssueParams) error {
	issueData := make(map[string]any)

	if params.StatusID > 0 {
		issueData["status_id"] = params.StatusID
	}
	if params.AssignedToID > 0 {
		issueData["assigned_to_id"] = params.AssignedToID
	}
	if params.Notes != "" {
		issueData["notes"] = params.Notes
	}

	if len(params.CustomFields) > 0 {
		customFields := make([]map[string]any, 0)
		for id, value := range params.CustomFields {
			cfID, _ := strconv.Atoi(id)
			customFields = append(customFields, map[string]any{
				"id":    cfID,
				"value": value,
			})
		}
		issueData["custom_fields"] = customFields
	}

	reqBody := map[string]any{
		"issue": issueData,
	}

	path := fmt.Sprintf("/issues/%d.json", params.IssueID)
	_, err := c.doRequest("PUT", path, reqBody)
	return err
}

// AddWatcher adds a watcher to an issue
func (c *Client) AddWatcher(issueID, userID int) error {
	reqBody := map[string]any{
		"user_id": userID,
	}

	path := fmt.Sprintf("/issues/%d/watchers.json", issueID)
	_, err := c.doRequest("POST", path, reqBody)
	return err
}

// CreateRelation creates a relation between issues
func (c *Client) CreateRelation(issueID, issueToID int, relationType string) (*Relation, error) {
	reqBody := map[string]any{
		"relation": map[string]any{
			"issue_to_id":   issueToID,
			"relation_type": relationType,
		},
	}

	path := fmt.Sprintf("/issues/%d/relations.json", issueID)
	data, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Relation Relation `json:"relation"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Relation, nil
}

// GetRelations returns relations for an issue
func (c *Client) GetRelations(issueID int) ([]Relation, error) {
	path := fmt.Sprintf("/issues/%d/relations.json", issueID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Relations []Relation `json:"relations"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Relations, nil
}

// CreateTimeEntryParams are parameters for creating a time entry
type CreateTimeEntryParams struct {
	IssueID    int
	Hours      float64
	ActivityID int
	Comments   string
}

// TimeEntry represents a time entry
type TimeEntry struct {
	ID        int     `json:"id"`
	Project   IDName  `json:"project"`
	Issue     *IDName `json:"issue,omitempty"`
	User      IDName  `json:"user"`
	Activity  IDName  `json:"activity"`
	Hours     float64 `json:"hours"`
	Comments  string  `json:"comments"`
	SpentOn   string  `json:"spent_on"`
	CreatedOn string  `json:"created_on"`
	UpdatedOn string  `json:"updated_on"`
}

// ListTimeEntriesParams are parameters for listing time entries
type ListTimeEntriesParams struct {
	ProjectID string
	UserID    string
	IssueID   int
	From      string // YYYY-MM-DD
	To        string // YYYY-MM-DD
	Limit     int
	Offset    int
}

// TimeEntriesResponse is the response from /time_entries.json
type TimeEntriesResponse struct {
	TimeEntries []TimeEntry `json:"time_entries"`
	TotalCount  int         `json:"total_count"`
	Offset      int         `json:"offset"`
	Limit       int         `json:"limit"`
}

// CreateTimeEntry creates a new time entry
func (c *Client) CreateTimeEntry(params CreateTimeEntryParams) (*TimeEntry, error) {
	reqBody := map[string]any{
		"time_entry": map[string]any{
			"issue_id": params.IssueID,
			"hours":    params.Hours,
		},
	}

	timeEntryData := reqBody["time_entry"].(map[string]any)

	if params.ActivityID > 0 {
		timeEntryData["activity_id"] = params.ActivityID
	}
	if params.Comments != "" {
		timeEntryData["comments"] = params.Comments
	}

	data, err := c.doRequest("POST", "/time_entries.json", reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		TimeEntry TimeEntry `json:"time_entry"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.TimeEntry, nil
}

// ListTimeEntries returns time entries with optional filters
func (c *Client) ListTimeEntries(params ListTimeEntriesParams) ([]TimeEntry, int, error) {
	query := url.Values{}

	if params.ProjectID != "" {
		query.Set("project_id", params.ProjectID)
	}
	if params.UserID != "" {
		query.Set("user_id", params.UserID)
	}
	if params.IssueID > 0 {
		query.Set("issue_id", strconv.Itoa(params.IssueID))
	}
	if params.From != "" {
		query.Set("from", params.From)
	}
	if params.To != "" {
		query.Set("to", params.To)
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	} else {
		query.Set("limit", "25")
	}
	if params.Offset > 0 {
		query.Set("offset", strconv.Itoa(params.Offset))
	}

	path := "/time_entries.json?" + query.Encode()
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, 0, err
	}

	var resp TimeEntriesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.TimeEntries, resp.TotalCount, nil
}

// ProjectMembership represents a project membership
type ProjectMembership struct {
	ID      int     `json:"id"`
	Project IDName  `json:"project"`
	User    *IDName `json:"user,omitempty"`
	Group   *IDName `json:"group,omitempty"`
	Roles   []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"roles"`
}

// GetProjectMemberships returns memberships for a project
func (c *Client) GetProjectMemberships(projectID int, limit int) ([]ProjectMembership, error) {
	if limit <= 0 {
		limit = 100
	}

	path := fmt.Sprintf("/projects/%d/memberships.json?limit=%d", projectID, limit)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Memberships []ProjectMembership `json:"memberships"`
		TotalCount  int                 `json:"total_count"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Memberships, nil
}

// GetProjectCustomFields returns custom fields available for a project/tracker
// by examining issues in that project (works without admin rights)
func (c *Client) GetProjectCustomFields(projectID int, trackerID int) ([]CustomFieldDefinition, error) {
	// Search for an issue in this project/tracker to extract custom field definitions
	query := url.Values{}
	query.Set("project_id", strconv.Itoa(projectID))
	if trackerID > 0 {
		query.Set("tracker_id", strconv.Itoa(trackerID))
	}
	query.Set("limit", "1")

	path := "/issues.json?" + query.Encode()
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp IssuesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.Issues) == 0 {
		return nil, fmt.Errorf("no issues found in project to extract custom fields")
	}

	// Get full issue details to see custom fields
	issue, err := c.GetIssue(resp.Issues[0].ID)
	if err != nil {
		return nil, err
	}

	// Convert custom field values to definitions
	definitions := make([]CustomFieldDefinition, len(issue.CustomFields))
	for i, cf := range issue.CustomFields {
		definitions[i] = CustomFieldDefinition{
			ID:          cf.ID,
			Name:        cf.Name,
			FieldFormat: "unknown", // We can't determine this from issue data
		}
	}

	return definitions, nil
}
