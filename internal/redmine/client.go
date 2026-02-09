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

// BaseURL returns the Redmine base URL
func (c *Client) BaseURL() string {
	return c.baseURL
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

// ListProjects returns all projects with pagination support
func (c *Client) ListProjects(limit int) ([]Project, error) {
	if limit <= 0 {
		limit = 100
	}

	var allProjects []Project
	offset := 0
	batchSize := 100 // Redmine typically limits to 100 per request

	for {
		path := fmt.Sprintf("/projects.json?limit=%d&offset=%d", batchSize, offset)
		data, err := c.doRequest("GET", path, nil)
		if err != nil {
			return nil, err
		}

		var resp ProjectsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		allProjects = append(allProjects, resp.Projects...)

		// Check if we've fetched all projects or reached the requested limit
		// Use total_count for accurate pagination detection
		if len(allProjects) >= resp.TotalCount || len(allProjects) >= limit || len(resp.Projects) == 0 {
			break
		}

		offset += len(resp.Projects)
	}

	// Trim to requested limit if we fetched more
	if len(allProjects) > limit {
		allProjects = allProjects[:limit]
	}

	return allProjects, nil
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

// IssuePriority represents a Redmine issue priority
type IssuePriority struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Active    bool   `json:"active"`
}

// ListIssuePriorities returns all issue priorities
func (c *Client) ListIssuePriorities() ([]IssuePriority, error) {
	data, err := c.doRequest("GET", "/enumerations/issue_priorities.json", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		IssuePriorities []IssuePriority `json:"issue_priorities"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.IssuePriorities, nil
}

// Role represents a Redmine role
type Role struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ListRoles returns all roles
func (c *Client) ListRoles() ([]Role, error) {
	data, err := c.doRequest("GET", "/roles.json", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Roles []Role `json:"roles"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Roles, nil
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

// CustomFieldDefinitionFull represents a full custom field definition from admin API
type CustomFieldDefinitionFull struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	CustomizedType string `json:"customized_type"`
	FieldFormat    string `json:"field_format"`
	IsRequired     bool   `json:"is_required"`
	Multiple       bool   `json:"multiple"`
	Visible        bool   `json:"visible"`
	PossibleValues []struct {
		Value string `json:"value"`
	} `json:"possible_values,omitempty"`
	Trackers     []IDName `json:"trackers,omitempty"`
	DefaultValue string   `json:"default_value,omitempty"`
}

// ProjectDetail represents a project with detailed includes (trackers, custom fields)
type ProjectDetail struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	Parent      *struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"parent,omitempty"`
	Trackers          []IDName `json:"trackers,omitempty"`
	IssueCustomFields []IDName `json:"issue_custom_fields,omitempty"`
}

// UpdateProjectParams are parameters for updating a project
type UpdateProjectParams struct {
	ProjectID           int
	Name                string // optional, omit if empty
	Description         string // optional, omit if empty
	TrackerIDs          []int  // nil = don't change, [] = clear all
	IssueCustomFieldIDs []int  // nil = don't change, [] = clear all
}

// Attachment represents a Redmine file attachment
type Attachment struct {
	ID          int    `json:"id"`
	Filename    string `json:"filename"`
	Filesize    int    `json:"filesize"`
	ContentType string `json:"content_type"`
	Description string `json:"description"`
	CreatedOn   string `json:"created_on"`
	Author      IDName `json:"author"`
}

// UploadToken represents an uploaded file token for attaching to issues
type UploadToken struct {
	Token       string `json:"token"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Description string `json:"description,omitempty"`
}

// Issue represents a Redmine issue
type Issue struct {
	ID           int     `json:"id"`
	Project      IDName  `json:"project"`
	Tracker      IDName  `json:"tracker"`
	Status       IDName  `json:"status"`
	Priority     IDName  `json:"priority"`
	Author       IDName  `json:"author"`
	AssignedTo   *IDName `json:"assigned_to,omitempty"`
	FixedVersion *IDName `json:"fixed_version,omitempty"`
	Subject      string  `json:"subject"`
	Description  string  `json:"description"`
	StartDate    string  `json:"start_date,omitempty"`
	DueDate      string  `json:"due_date,omitempty"`
	DoneRatio    int     `json:"done_ratio"`
	CreatedOn    string  `json:"created_on"`
	UpdatedOn    string  `json:"updated_on"`
	ClosedOn     string  `json:"closed_on,omitempty"`
	Parent       *struct {
		ID int `json:"id"`
	} `json:"parent,omitempty"`
	CustomFields    []CustomField `json:"custom_fields,omitempty"`
	Journals        []Journal     `json:"journals,omitempty"`
	Watchers        []IDName      `json:"watchers,omitempty"`
	Relations       []Relation    `json:"relations,omitempty"`
	AllowedStatuses []IDName      `json:"allowed_statuses,omitempty"`
	Attachments     []Attachment  `json:"attachments,omitempty"`
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
	VersionID    string // Version/milestone ID or name
	Subject      string // Search keyword for subject (partial match)
	ParentID          int
	CreatedOn         string // Redmine date filter, e.g., ">=2024-01-01"
	UpdatedOn         string // Redmine date filter, e.g., ">=2024-01-01"
	Sort              string // Sort order, e.g., "updated_on:desc"
	CustomFieldFilter map[string]string // cf_ID -> value
	Limit             int
	Offset            int
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
	if params.VersionID != "" {
		query.Set("fixed_version_id", params.VersionID)
	}
	if params.Subject != "" {
		// Use ~keyword for partial match (contains)
		query.Set("subject", "~"+params.Subject)
	}
	if params.ParentID > 0 {
		query.Set("parent_id", strconv.Itoa(params.ParentID))
	}
	if params.CreatedOn != "" {
		query.Set("created_on", params.CreatedOn)
	}
	if params.UpdatedOn != "" {
		query.Set("updated_on", params.UpdatedOn)
	}
	if params.Sort != "" {
		query.Set("sort", params.Sort)
	}
	for cfID, value := range params.CustomFieldFilter {
		query.Set("cf_"+cfID, value)
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
	path := fmt.Sprintf("/issues/%d.json?include=journals,watchers,relations,allowed_statuses,attachments", issueID)
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
	IsPrivate     *bool
	CustomFields  map[string]any
	Uploads       []UploadToken
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
	if params.IsPrivate != nil {
		issueData["is_private"] = *params.IsPrivate
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

	if len(params.Uploads) > 0 {
		uploads := make([]map[string]any, len(params.Uploads))
		for i, u := range params.Uploads {
			upload := map[string]any{
				"token":    u.Token,
				"filename": u.Filename,
			}
			if u.ContentType != "" {
				upload["content_type"] = u.ContentType
			}
			if u.Description != "" {
				upload["description"] = u.Description
			}
			uploads[i] = upload
		}
		issueData["uploads"] = uploads
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
	Subject      string
	Description  string
	StatusID     int
	PriorityID   int
	TrackerID    int
	AssignedToID int
	StartDate    string
	DueDate      string
	DoneRatio    *int // nil = don't change, 0-100 = set value
	IsPrivate    *bool
	Notes        string
	CustomFields map[string]any
	Uploads      []UploadToken
}

// UpdateIssue updates an existing issue
func (c *Client) UpdateIssue(params UpdateIssueParams) error {
	issueData := make(map[string]any)

	if params.Subject != "" {
		issueData["subject"] = params.Subject
	}
	if params.Description != "" {
		issueData["description"] = params.Description
	}
	if params.StatusID > 0 {
		issueData["status_id"] = params.StatusID
	}
	if params.PriorityID > 0 {
		issueData["priority_id"] = params.PriorityID
	}
	if params.TrackerID > 0 {
		issueData["tracker_id"] = params.TrackerID
	}
	if params.AssignedToID > 0 {
		issueData["assigned_to_id"] = params.AssignedToID
	}
	if params.StartDate != "" {
		issueData["start_date"] = params.StartDate
	}
	if params.DueDate != "" {
		issueData["due_date"] = params.DueDate
	}
	if params.DoneRatio != nil {
		issueData["done_ratio"] = *params.DoneRatio
	}
	if params.IsPrivate != nil {
		issueData["is_private"] = *params.IsPrivate
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

	if len(params.Uploads) > 0 {
		uploads := make([]map[string]any, len(params.Uploads))
		for i, u := range params.Uploads {
			upload := map[string]any{
				"token":    u.Token,
				"filename": u.Filename,
			}
			if u.ContentType != "" {
				upload["content_type"] = u.ContentType
			}
			if u.Description != "" {
				upload["description"] = u.Description
			}
			uploads[i] = upload
		}
		issueData["uploads"] = uploads
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
	SpentOn    string // Date in YYYY-MM-DD format
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
	if params.SpentOn != "" {
		timeEntryData["spent_on"] = params.SpentOn
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
	// If no date filter specified, use spent_on=* to get all entries
	// (Redmine defaults to current month otherwise)
	if params.From == "" && params.To == "" {
		query.Set("spent_on", "*")
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

// CreateProjectMembership adds a user or group to a project with specified roles
func (c *Client) CreateProjectMembership(projectID int, userID *int, groupID *int, roleIDs []int) (*ProjectMembership, error) {
	if (userID == nil && groupID == nil) || (userID != nil && groupID != nil) {
		return nil, fmt.Errorf("must specify exactly one of user_id or group_id")
	}
	if len(roleIDs) == 0 {
		return nil, fmt.Errorf("must specify at least one role")
	}

	membership := map[string]any{
		"role_ids": roleIDs,
	}
	if userID != nil {
		membership["user_id"] = *userID
	}
	if groupID != nil {
		membership["group_id"] = *groupID
	}

	payload := map[string]any{
		"membership": membership,
	}

	path := fmt.Sprintf("/projects/%d/memberships.json", projectID)
	data, err := c.doRequest("POST", path, payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Membership ProjectMembership `json:"membership"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Membership, nil
}

// UpdateProjectMembership updates the roles for a membership
func (c *Client) UpdateProjectMembership(membershipID int, roleIDs []int) error {
	if len(roleIDs) == 0 {
		return fmt.Errorf("must specify at least one role")
	}

	payload := map[string]any{
		"membership": map[string]any{
			"role_ids": roleIDs,
		},
	}

	path := fmt.Sprintf("/memberships/%d.json", membershipID)
	_, err := c.doRequest("PUT", path, payload)
	return err
}

// DeleteProjectMembership removes a membership from a project
func (c *Client) DeleteProjectMembership(membershipID int) error {
	path := fmt.Sprintf("/memberships/%d.json", membershipID)
	_, err := c.doRequest("DELETE", path, nil)
	return err
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

// ListAllCustomFields returns all custom field definitions (requires admin privileges)
func (c *Client) ListAllCustomFields() ([]CustomFieldDefinitionFull, error) {
	data, err := c.doRequest("GET", "/custom_fields.json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom fields (requires admin privileges): %w", err)
	}

	var resp struct {
		CustomFields []CustomFieldDefinitionFull `json:"custom_fields"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.CustomFields, nil
}

// GetProjectDetail returns a project with detailed includes (trackers, custom fields)
func (c *Client) GetProjectDetail(projectID int, includes []string) (*ProjectDetail, error) {
	path := fmt.Sprintf("/projects/%d.json", projectID)
	if len(includes) > 0 {
		path += "?include=" + strings.Join(includes, ",")
	}

	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Project ProjectDetail `json:"project"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Project, nil
}

// doRequestRaw performs an HTTP request with a raw body (non-JSON)
func (c *Client) doRequestRaw(method, path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Redmine-API-Key", c.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

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

// UploadFile uploads a file to Redmine and returns an upload token
func (c *Client) UploadFile(filename string, content io.Reader) (*UploadToken, error) {
	data, err := c.doRequestRaw("POST", "/uploads.json", content, "application/octet-stream")
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	var resp struct {
		Upload struct {
			Token string `json:"token"`
		} `json:"upload"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w", err)
	}

	return &UploadToken{
		Token:    resp.Upload.Token,
		Filename: filename,
	}, nil
}

// GetAttachment returns attachment metadata by ID
func (c *Client) GetAttachment(id int) (*Attachment, error) {
	path := fmt.Sprintf("/attachments/%d.json", id)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Attachment Attachment `json:"attachment"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Attachment, nil
}

// DownloadAttachment downloads an attachment's content by ID.
// Returns the file bytes, content type, and filename.
func (c *Client) DownloadAttachment(id int) ([]byte, string, string, error) {
	// First get the attachment metadata to find the download URL and filename
	attachment, err := c.GetAttachment(id)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get attachment info: %w", err)
	}

	// Download via /attachments/download/{id}/{filename}
	path := fmt.Sprintf("/attachments/download/%d/%s", id, attachment.Filename)
	data, err := c.doRequestRaw("GET", path, nil, "")
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to download attachment: %w", err)
	}

	return data, attachment.ContentType, attachment.Filename, nil
}

// UpdateTimeEntryParams are parameters for updating a time entry
type UpdateTimeEntryParams struct {
	TimeEntryID int
	Hours       float64
	ActivityID  int
	Comments    string
	SpentOn     string // Date in YYYY-MM-DD format
}

// UpdateTimeEntry updates an existing time entry
func (c *Client) UpdateTimeEntry(params UpdateTimeEntryParams) error {
	timeEntryData := make(map[string]any)

	if params.Hours > 0 {
		timeEntryData["hours"] = params.Hours
	}
	if params.ActivityID > 0 {
		timeEntryData["activity_id"] = params.ActivityID
	}
	if params.Comments != "" {
		timeEntryData["comments"] = params.Comments
	}
	if params.SpentOn != "" {
		timeEntryData["spent_on"] = params.SpentOn
	}

	reqBody := map[string]any{
		"time_entry": timeEntryData,
	}

	path := fmt.Sprintf("/time_entries/%d.json", params.TimeEntryID)
	_, err := c.doRequest("PUT", path, reqBody)
	return err
}

// DeleteTimeEntry deletes a time entry
func (c *Client) DeleteTimeEntry(id int) error {
	path := fmt.Sprintf("/time_entries/%d.json", id)
	_, err := c.doRequest("DELETE", path, nil)
	return err
}

// RemoveWatcher removes a watcher from an issue
func (c *Client) RemoveWatcher(issueID, userID int) error {
	path := fmt.Sprintf("/issues/%d/watchers/%d.json", issueID, userID)
	_, err := c.doRequest("DELETE", path, nil)
	return err
}

// DeleteRelation deletes an issue relation
func (c *Client) DeleteRelation(relationID int) error {
	path := fmt.Sprintf("/relations/%d.json", relationID)
	_, err := c.doRequest("DELETE", path, nil)
	return err
}

// SearchUsersParams are parameters for searching users
type SearchUsersParams struct {
	Name      string
	ProjectID int
	Status    int // 1=active, 2=registered, 3=locked
	Limit     int
	Offset    int
}

// SearchUsers searches for users (tries admin API first, falls back to memberships)
func (c *Client) SearchUsers(params SearchUsersParams) ([]User, int, error) {
	// Try admin API first: GET /users.json
	query := url.Values{}
	if params.Name != "" {
		query.Set("name", params.Name)
	}
	if params.Status > 0 {
		query.Set("status", strconv.Itoa(params.Status))
	} else {
		query.Set("status", "1") // default to active
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	} else {
		query.Set("limit", "25")
	}
	if params.Offset > 0 {
		query.Set("offset", strconv.Itoa(params.Offset))
	}

	path := "/users.json?" + query.Encode()
	data, err := c.doRequest("GET", path, nil)
	if err == nil {
		var resp struct {
			Users      []User `json:"users"`
			TotalCount int    `json:"total_count"`
		}
		if parseErr := json.Unmarshal(data, &resp); parseErr == nil {
			// Fill in Name if empty
			for i := range resp.Users {
				if resp.Users[i].Name == "" {
					resp.Users[i].Name = resp.Users[i].Firstname + " " + resp.Users[i].Lastname
				}
			}
			return resp.Users, resp.TotalCount, nil
		}
	}

	// Fallback: use project memberships if admin API fails (403)
	if params.ProjectID == 0 {
		return nil, 0, fmt.Errorf("user search requires admin privileges or a project context (provide project parameter)")
	}

	memberships, mErr := c.GetProjectMemberships(params.ProjectID, 100)
	if mErr != nil {
		return nil, 0, fmt.Errorf("failed to search users (admin API unavailable, membership fallback failed): %w", mErr)
	}

	var users []User
	nameFilter := strings.ToLower(params.Name)
	for _, m := range memberships {
		if m.User == nil {
			continue
		}
		if nameFilter != "" && !strings.Contains(strings.ToLower(m.User.Name), nameFilter) {
			continue
		}
		users = append(users, User{
			ID:   m.User.ID,
			Name: m.User.Name,
		})
	}

	return users, len(users), nil
}

// Version represents a Redmine version/milestone
type Version struct {
	ID          int    `json:"id"`
	Project     IDName `json:"project"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"`
	Sharing     string `json:"sharing"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
}

// ListVersions returns all versions for a project
func (c *Client) ListVersions(projectID int) ([]Version, error) {
	path := fmt.Sprintf("/projects/%d/versions.json", projectID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Versions []Version `json:"versions"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Versions, nil
}

// CreateVersionParams are parameters for creating a version
type CreateVersionParams struct {
	ProjectID   int
	Name        string
	Description string
	Status      string // open (default), locked, closed
	DueDate     string
	Sharing     string // none (default), descendants, hierarchy, tree, system
}

// CreateVersion creates a new version in a project
func (c *Client) CreateVersion(params CreateVersionParams) (*Version, error) {
	versionData := map[string]any{
		"name": params.Name,
	}
	if params.Description != "" {
		versionData["description"] = params.Description
	}
	if params.Status != "" {
		versionData["status"] = params.Status
	}
	if params.DueDate != "" {
		versionData["due_date"] = params.DueDate
	}
	if params.Sharing != "" {
		versionData["sharing"] = params.Sharing
	}

	reqBody := map[string]any{
		"version": versionData,
	}

	path := fmt.Sprintf("/projects/%d/versions.json", params.ProjectID)
	data, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Version Version `json:"version"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.Version, nil
}

// UpdateVersionParams are parameters for updating a version
type UpdateVersionParams struct {
	VersionID   int
	Name        string
	Description string
	Status      string // open, locked, closed
	DueDate     string
	Sharing     string // none, descendants, hierarchy, tree, system
}

// UpdateVersion updates an existing version
func (c *Client) UpdateVersion(params UpdateVersionParams) error {
	versionData := make(map[string]any)

	if params.Name != "" {
		versionData["name"] = params.Name
	}
	if params.Description != "" {
		versionData["description"] = params.Description
	}
	if params.Status != "" {
		versionData["status"] = params.Status
	}
	if params.DueDate != "" {
		versionData["due_date"] = params.DueDate
	}
	if params.Sharing != "" {
		versionData["sharing"] = params.Sharing
	}

	reqBody := map[string]any{
		"version": versionData,
	}

	path := fmt.Sprintf("/versions/%d.json", params.VersionID)
	_, err := c.doRequest("PUT", path, reqBody)
	return err
}

// IssueCategory represents an issue category in a project
type IssueCategory struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Project        IDName `json:"project"`
	AssignedTo     IDName `json:"assigned_to,omitempty"`
	AssignedToID   int    `json:"-"` // For create/update
}

// ListIssueCategories returns all issue categories for a project
func (c *Client) ListIssueCategories(projectID int) ([]IssueCategory, error) {
	path := fmt.Sprintf("/projects/%d/issue_categories.json", projectID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		IssueCategories []IssueCategory `json:"issue_categories"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.IssueCategories, nil
}

// CreateIssueCategoryParams are parameters for creating an issue category
type CreateIssueCategoryParams struct {
	ProjectID    int
	Name         string
	AssignedToID int // Optional: default assignee for issues in this category
}

// CreateIssueCategory creates a new issue category in a project
func (c *Client) CreateIssueCategory(params CreateIssueCategoryParams) (*IssueCategory, error) {
	categoryData := map[string]any{
		"name": params.Name,
	}
	if params.AssignedToID > 0 {
		categoryData["assigned_to_id"] = params.AssignedToID
	}

	reqBody := map[string]any{
		"issue_category": categoryData,
	}

	path := fmt.Sprintf("/projects/%d/issue_categories.json", params.ProjectID)
	data, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		IssueCategory IssueCategory `json:"issue_category"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.IssueCategory, nil
}

// UpdateIssueCategoryParams are parameters for updating an issue category
type UpdateIssueCategoryParams struct {
	CategoryID   int
	Name         string
	AssignedToID int
}

// UpdateIssueCategory updates an existing issue category
func (c *Client) UpdateIssueCategory(params UpdateIssueCategoryParams) error {
	categoryData := make(map[string]any)

	if params.Name != "" {
		categoryData["name"] = params.Name
	}
	if params.AssignedToID > 0 {
		categoryData["assigned_to_id"] = params.AssignedToID
	}

	if len(categoryData) == 0 {
		return nil // Nothing to update
	}

	reqBody := map[string]any{
		"issue_category": categoryData,
	}

	path := fmt.Sprintf("/issue_categories/%d.json", params.CategoryID)
	_, err := c.doRequest("PUT", path, reqBody)
	return err
}

// DeleteIssueCategory deletes an issue category
func (c *Client) DeleteIssueCategory(categoryID int) error {
	path := fmt.Sprintf("/issue_categories/%d.json", categoryID)
	_, err := c.doRequest("DELETE", path, nil)
	return err
}

// WikiPage represents a wiki page in the index
type WikiPage struct {
	Title     string `json:"title"`
	Version   int    `json:"version"`
	CreatedOn string `json:"created_on"`
	UpdatedOn string `json:"updated_on"`
}

// WikiPageDetail represents a wiki page with content
type WikiPageDetail struct {
	Title     string `json:"title"`
	Text      string `json:"text"`
	Version   int    `json:"version"`
	Author    IDName `json:"author"`
	Comments  string `json:"comments"`
	CreatedOn string `json:"created_on"`
	UpdatedOn string `json:"updated_on"`
}

// ListWikiPages returns all wiki pages for a project
func (c *Client) ListWikiPages(projectID int) ([]WikiPage, error) {
	path := fmt.Sprintf("/projects/%d/wiki/index.json", projectID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		WikiPages []WikiPage `json:"wiki_pages"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.WikiPages, nil
}

// GetWikiPage returns a wiki page by title
func (c *Client) GetWikiPage(projectID int, title string) (*WikiPageDetail, error) {
	path := fmt.Sprintf("/projects/%d/wiki/%s.json", projectID, url.PathEscape(title))
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		WikiPage WikiPageDetail `json:"wiki_page"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.WikiPage, nil
}

// WikiPageParams are parameters for creating or updating a wiki page
type WikiPageParams struct {
	ProjectID int
	Title     string
	Text      string
	Comments  string // edit comment / version note
}

// CreateOrUpdateWikiPage creates or updates a wiki page
func (c *Client) CreateOrUpdateWikiPage(params WikiPageParams) error {
	wikiData := map[string]any{
		"text": params.Text,
	}
	if params.Comments != "" {
		wikiData["comments"] = params.Comments
	}

	reqBody := map[string]any{
		"wiki_page": wikiData,
	}

	path := fmt.Sprintf("/projects/%d/wiki/%s.json", params.ProjectID, url.PathEscape(params.Title))
	_, err := c.doRequest("PUT", path, reqBody)
	return err
}

// UpdateProject updates a project's settings
func (c *Client) UpdateProject(params UpdateProjectParams) error {
	projectData := make(map[string]any)

	if params.Name != "" {
		projectData["name"] = params.Name
	}
	if params.Description != "" {
		projectData["description"] = params.Description
	}
	if params.TrackerIDs != nil {
		projectData["tracker_ids"] = params.TrackerIDs
	}
	if params.IssueCustomFieldIDs != nil {
		projectData["issue_custom_field_ids"] = params.IssueCustomFieldIDs
	}

	reqBody := map[string]any{
		"project": projectData,
	}

	path := fmt.Sprintf("/projects/%d.json", params.ProjectID)
	_, err := c.doRequest("PUT", path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to update project (may require admin or project manager privileges): %w", err)
	}
	return nil
}

// ProjectFile represents a file in the project Files section
type ProjectFile struct {
	ID          int    `json:"id"`
	Filename    string `json:"filename"`
	Filesize    int    `json:"filesize"`
	ContentType string `json:"content_type"`
	Description string `json:"description"`
	ContentURL  string `json:"content_url"`
	Author      IDName `json:"author"`
	CreatedOn   string `json:"created_on"`
	Version     *IDName `json:"version,omitempty"`
	Downloads   int    `json:"downloads"`
	Digest      string `json:"digest"`
}

// ListProjectFiles returns all files for a project
func (c *Client) ListProjectFiles(projectID int) ([]ProjectFile, error) {
	path := fmt.Sprintf("/projects/%d/files.json", projectID)
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Files []ProjectFile `json:"files"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Files, nil
}

// CreateProjectFileParams are parameters for creating a project file
type CreateProjectFileParams struct {
	ProjectID   int
	Token       string // Upload token from UploadFile
	Filename    string // Optional: override filename
	Description string // Optional: file description
	VersionID   int    // Optional: associate with version
}

// CreateProjectFile creates a new file in the project Files section
func (c *Client) CreateProjectFile(params CreateProjectFileParams) (*ProjectFile, error) {
	fileData := map[string]any{
		"token": params.Token,
	}
	if params.Filename != "" {
		fileData["filename"] = params.Filename
	}
	if params.Description != "" {
		fileData["description"] = params.Description
	}
	if params.VersionID > 0 {
		fileData["version_id"] = params.VersionID
	}

	reqBody := map[string]any{
		"file": fileData,
	}

	path := fmt.Sprintf("/projects/%d/files.json", params.ProjectID)
	data, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		File ProjectFile `json:"file"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp.File, nil
}

// SearchResult represents a result from Redmine global search
type SearchResult struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Datetime    string `json:"datetime"`
}

// GlobalSearchParams are parameters for global search across all Redmine resources
type GlobalSearchParams struct {
	Query      string
	Scope      string // "all", "my_projects", "subprojects"
	AllWords   bool
	TitlesOnly bool
	Issues     bool
	News       bool
	Documents  bool
	Changesets bool
	WikiPages  bool
	Messages   bool
	Projects   bool
	Offset     int
	Limit      int
}

// GlobalSearch searches across all Redmine resources (issues, wiki, news, etc.)
func (c *Client) GlobalSearch(params GlobalSearchParams) ([]SearchResult, int, error) {
	query := url.Values{}
	query.Set("q", params.Query)

	if params.Scope != "" {
		query.Set("scope", params.Scope)
	}
	if params.AllWords {
		query.Set("all_words", "1")
	}
	if params.TitlesOnly {
		query.Set("titles_only", "1")
	}

	// Resource type filters — Redmine uses these as toggles
	if params.Issues {
		query.Set("issues", "1")
	}
	if params.News {
		query.Set("news", "1")
	}
	if params.Documents {
		query.Set("documents", "1")
	}
	if params.Changesets {
		query.Set("changesets", "1")
	}
	if params.WikiPages {
		query.Set("wiki_pages", "1")
	}
	if params.Messages {
		query.Set("messages", "1")
	}
	if params.Projects {
		query.Set("projects", "1")
	}

	if params.Offset > 0 {
		query.Set("offset", strconv.Itoa(params.Offset))
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	} else {
		query.Set("limit", "25")
	}

	path := "/search.json?" + query.Encode()
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, 0, err
	}

	var resp struct {
		Results    []SearchResult `json:"results"`
		TotalCount int            `json:"total_count"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Results, resp.TotalCount, nil
}

// DMSFFile represents a file in DMSF
type DMSFFile struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	Size        int    `json:"size"`
	ContentURL  string `json:"content_url"`
}

// DMSFUploadResponse is the response from DMSF upload
type DMSFUploadResponse struct {
	Upload struct {
		Token string `json:"token"`
	} `json:"upload"`
}

// DMSFUploadFile uploads a file to DMSF and returns a token
func (c *Client) DMSFUploadFile(projectID int, filename string, content io.Reader) (string, error) {
	// Read content
	data, err := io.ReadAll(content)
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	// Build URL with filename
	path := fmt.Sprintf("/projects/%d/dmsf/upload.json?filename=%s", projectID, url.QueryEscape(filename))

	// Make request with binary content
	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Redmine-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("DMSF upload failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var uploadResp DMSFUploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return uploadResp.Upload.Token, nil
}

// DMSFCommitParams are parameters for committing a file to DMSF
type DMSFCommitParams struct {
	ProjectID   int
	Token       string
	Filename    string
	Title       string
	Description string
	Comment     string
	FolderID    int // Optional: 0 means root folder
}

// DMSFCommitFile commits an uploaded file to DMSF
func (c *Client) DMSFCommitFile(params DMSFCommitParams) (*DMSFFile, error) {
	uploadedFile := map[string]any{
		"name":  params.Filename,
		"token": params.Token,
	}
	if params.Title != "" {
		uploadedFile["title"] = params.Title
	}
	if params.Description != "" {
		uploadedFile["description"] = params.Description
	}
	if params.Comment != "" {
		uploadedFile["comment"] = params.Comment
	}

	attachments := map[string]any{
		"uploaded_file": uploadedFile,
	}
	if params.FolderID > 0 {
		attachments["folder_id"] = params.FolderID
	}

	reqBody := map[string]any{
		"attachments": attachments,
	}

	path := fmt.Sprintf("/projects/%d/dmsf/commit.json", params.ProjectID)
	data, err := c.doRequest("POST", path, reqBody)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Files []DMSFFile `json:"dmsf_files"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.Files) == 0 {
		return nil, fmt.Errorf("no file created in response")
	}

	return &resp.Files[0], nil
}

// DMSFCreateFileParams combines upload and commit for convenience
type DMSFCreateFileParams struct {
	ProjectID   int
	Filename    string
	Title       string
	Description string
	Comment     string
	FolderID    int
	Content     io.Reader
}

// DMSFCreateFile uploads and commits a file to DMSF in one call
func (c *Client) DMSFCreateFile(params DMSFCreateFileParams) (*DMSFFile, error) {
	// Step 1: Upload
	token, err := c.DMSFUploadFile(params.ProjectID, params.Filename, params.Content)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	// Step 2: Commit
	commitParams := DMSFCommitParams{
		ProjectID:   params.ProjectID,
		Token:       token,
		Filename:    params.Filename,
		Title:       params.Title,
		Description: params.Description,
		Comment:     params.Comment,
		FolderID:    params.FolderID,
	}

	file, err := c.DMSFCommitFile(commitParams)
	if err != nil {
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	return file, nil
}
