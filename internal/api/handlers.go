package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

// @title Redmine MCP Server API
// @version 1.0
// @description REST API for Redmine integration with AI assistants
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-Redmine-API-Key

type contextKey string

const clientContextKey contextKey = "redmineClient"

func withClient(ctx context.Context, client *redmine.Client) context.Context {
	return context.WithValue(ctx, clientContextKey, client)
}

func getClient(ctx context.Context) *redmine.Client {
	return ctx.Value(clientContextKey).(*redmine.Client)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// @Summary Get current user
// @Description Returns information about the current user
// @Tags Account
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Router /me [get]
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	user, err := client.GetCurrentUser()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":        user.ID,
		"login":     user.Login,
		"firstname": user.Firstname,
		"lastname":  user.Lastname,
		"name":      user.Firstname + " " + user.Lastname,
		"email":     user.Mail,
	})
}

// @Summary List projects
// @Description Returns a list of all projects
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param limit query int false "Number of projects to return" default(100)
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Router /projects [get]
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}

	projects, err := client.ListProjects(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(projects))
	for i, p := range projects {
		result[i] = map[string]any{
			"id":          p.ID,
			"name":        p.Name,
			"identifier":  p.Identifier,
			"description": p.Description,
		}
		if p.Parent != nil {
			result[i]["parent"] = map[string]any{
				"id":   p.Parent.ID,
				"name": p.Parent.Name,
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"projects": result,
		"count":    len(projects),
	})
}

// @Summary Create project
// @Description Create a new project
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body object true "Project data"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects [post]
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	var req struct {
		Name        string `json:"name"`
		Identifier  string `json:"identifier"`
		Description string `json:"description"`
		Parent      string `json:"parent"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" || req.Identifier == "" {
		writeError(w, http.StatusBadRequest, "name and identifier are required")
		return
	}

	var parentID int
	if req.Parent != "" {
		var err error
		parentID, err = resolver.ResolveProject(req.Parent)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	project, err := client.CreateProject(req.Name, req.Identifier, req.Description, parentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         project.ID,
		"name":       project.Name,
		"identifier": project.Identifier,
	})
}

// @Summary Search issues
// @Description Search issues with filters
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param project query string false "Project name or ID"
// @Param tracker query string false "Tracker name or ID"
// @Param status query string false "Status: open, closed, all, or specific name"
// @Param assigned_to query string false "Assignee name or 'me'"
// @Param limit query int false "Number of issues to return" default(25)
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues [get]
func (s *Server) handleSearchIssues(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	params := redmine.SearchIssuesParams{}
	q := r.URL.Query()

	if project := q.Get("project"); project != "" {
		projectID, err := resolver.ResolveProject(project)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.ProjectID = strconv.Itoa(projectID)
	}

	if tracker := q.Get("tracker"); tracker != "" {
		trackerID, err := resolver.ResolveTracker(tracker)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.TrackerID = trackerID
	}

	if status := q.Get("status"); status != "" {
		statusID, err := resolver.ResolveStatus(status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.StatusID = statusID
	} else {
		params.StatusID = "open"
	}

	if assignedTo := q.Get("assigned_to"); assignedTo != "" {
		if assignedTo == "me" {
			params.AssignedToID = "me"
		} else {
			var projectID int
			if params.ProjectID != "" {
				projectID, _ = strconv.Atoi(params.ProjectID)
			}
			userID, err := resolver.ResolveUser(assignedTo, projectID)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			params.AssignedToID = strconv.Itoa(userID)
		}
	}

	if parentID := q.Get("parent_id"); parentID != "" {
		if n, err := strconv.Atoi(parentID); err == nil {
			params.ParentID = n
		}
	}

	if after := q.Get("updated_after"); after != "" {
		params.UpdatedOn = ">=" + after
	}
	if before := q.Get("updated_before"); before != "" {
		if params.UpdatedOn != "" {
			after := q.Get("updated_after")
			params.UpdatedOn = "><" + after + "|" + before
		} else {
			params.UpdatedOn = "<=" + before
		}
	}
	if after := q.Get("created_after"); after != "" {
		params.CreatedOn = ">=" + after
	}
	if before := q.Get("created_before"); before != "" {
		if params.CreatedOn != "" {
			after := q.Get("created_after")
			params.CreatedOn = "><" + after + "|" + before
		} else {
			params.CreatedOn = "<=" + before
		}
	}

	params.Sort = q.Get("sort")

	if limit := q.Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			params.Limit = n
		}
	}
	if offset := q.Get("offset"); offset != "" {
		if n, err := strconv.Atoi(offset); err == nil {
			params.Offset = n
		}
	}

	issues, total, err := client.SearchIssues(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(issues))
	for i, issue := range issues {
		result[i] = formatIssueAPI(issue)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"issues":      result,
		"count":       len(issues),
		"total_count": total,
	})
}

// @Summary Get issue
// @Description Get issue details including journals and relations
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Issue ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id} [get]
func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	issue, err := client.GetIssue(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := formatIssueDetailAPI(*issue)

	// Add allowed_statuses from workflow rules (Redmine pre-5.0 doesn't provide this)
	if s.workflow != nil && len(issue.AllowedStatuses) == 0 {
		if allowed := s.workflow.GetAllowedStatuses(issue.Tracker.ID, issue.Status.ID); len(allowed) > 0 {
			statuses := make([]map[string]any, len(allowed))
			for i, as := range allowed {
				statuses[i] = map[string]any{"id": as.ID, "name": as.Name}
			}
			result["allowed_statuses"] = statuses
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// @Summary Create issue
// @Description Create a new issue
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body object true "Issue data"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues [post]
func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	var req struct {
		Project       string         `json:"project"`
		Tracker       string         `json:"tracker"`
		Subject       string         `json:"subject"`
		Description   string         `json:"description"`
		AssignedTo    string         `json:"assigned_to"`
		ParentIssueID int            `json:"parent_issue_id"`
		StartDate     string         `json:"start_date"`
		DueDate       string         `json:"due_date"`
		IsPrivate     *bool          `json:"is_private"`
		CustomFields  map[string]any `json:"custom_fields"`
		UploadTokens  []struct {
			Token       string `json:"token"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Description string `json:"description"`
		} `json:"upload_tokens"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Project == "" || req.Tracker == "" || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "project, tracker, and subject are required")
		return
	}

	projectID, err := resolver.ResolveProject(req.Project)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	trackerID, err := resolver.ResolveTracker(req.Tracker)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	params := redmine.CreateIssueParams{
		ProjectID:   projectID,
		TrackerID:   trackerID,
		Subject:     req.Subject,
		Description: req.Description,
	}

	if req.AssignedTo != "" {
		userID, err := resolver.ResolveUser(req.AssignedTo, projectID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.AssignedToID = userID
	}

	params.ParentIssueID = req.ParentIssueID
	params.StartDate = req.StartDate
	params.DueDate = req.DueDate
	params.IsPrivate = req.IsPrivate

	if req.CustomFields != nil {
		resolved, err := resolveCustomFieldsAPI(req.CustomFields, s.rules)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.CustomFields = resolved
	}

	for _, t := range req.UploadTokens {
		params.Uploads = append(params.Uploads, redmine.UploadToken{
			Token:       t.Token,
			Filename:    t.Filename,
			ContentType: t.ContentType,
			Description: t.Description,
		})
	}

	issue, err := client.CreateIssue(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, formatIssueAPI(*issue))
}

// @Summary Update issue
// @Description Update an existing issue
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Issue ID"
// @Param request body object true "Update data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id} [patch]
func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	var req struct {
		Subject      string         `json:"subject"`
		Description  string         `json:"description"`
		Status       string         `json:"status"`
		Priority     string         `json:"priority"`
		Tracker      string         `json:"tracker"`
		AssignedTo   string         `json:"assigned_to"`
		StartDate    string         `json:"start_date"`
		DueDate      string         `json:"due_date"`
		Notes        string         `json:"notes"`
		DoneRatio    *int           `json:"done_ratio"`
		IsPrivate    *bool          `json:"is_private"`
		CustomFields map[string]any `json:"custom_fields"`
		UploadTokens []struct {
			Token       string `json:"token"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Description string `json:"description"`
		} `json:"upload_tokens"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get issue for project context
	issue, err := client.GetIssue(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	params := redmine.UpdateIssueParams{
		IssueID:     id,
		Subject:     req.Subject,
		Description: req.Description,
		StartDate:   req.StartDate,
		DueDate:     req.DueDate,
		Notes:       req.Notes,
		DoneRatio:   req.DoneRatio,
		IsPrivate:   req.IsPrivate,
	}

	if req.Priority != "" {
		priorityID, err := resolver.ResolvePriority(req.Priority)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.PriorityID = priorityID
	}

	if req.Tracker != "" {
		trackerID, err := resolver.ResolveTracker(req.Tracker)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.TrackerID = trackerID
	}

	if req.Status != "" {
		statusID, err := resolver.ResolveStatusID(req.Status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.workflow.ValidateTransition(issue.Tracker.ID, issue.Status.ID, statusID); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid status transition: %v", err))
			return
		}
		params.StatusID = statusID
	}

	if req.AssignedTo != "" {
		userID, err := resolver.ResolveUser(req.AssignedTo, issue.Project.ID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.AssignedToID = userID
	}

	if req.CustomFields != nil {
		resolved, err := resolveCustomFieldsAPI(req.CustomFields, s.rules)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.CustomFields = resolved
	}

	for _, t := range req.UploadTokens {
		params.Uploads = append(params.Uploads, redmine.UploadToken{
			Token:       t.Token,
			Filename:    t.Filename,
			ContentType: t.ContentType,
			Description: t.Description,
		})
	}

	if err := client.UpdateIssue(params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"issue_id": id,
		"message":  "Issue updated successfully",
	})
}

// @Summary Create subtask
// @Description Create a subtask under an existing issue
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Parent Issue ID"
// @Param request body object true "Subtask data"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/subtasks [post]
func (s *Server) handleCreateSubtask(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	idStr := chi.URLParam(r, "id")
	parentID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	var req struct {
		Subject      string         `json:"subject"`
		Tracker      string         `json:"tracker"`
		Description  string         `json:"description"`
		AssignedTo   string         `json:"assigned_to"`
		CustomFields map[string]any `json:"custom_fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject is required")
		return
	}

	// Get parent issue
	parent, err := client.GetIssue(parentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	params := redmine.CreateIssueParams{
		ProjectID:     parent.Project.ID,
		TrackerID:     parent.Tracker.ID,
		Subject:       req.Subject,
		Description:   req.Description,
		ParentIssueID: parentID,
	}

	if req.Tracker != "" {
		trackerID, err := resolver.ResolveTracker(req.Tracker)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.TrackerID = trackerID
	}

	if req.AssignedTo != "" {
		userID, err := resolver.ResolveUser(req.AssignedTo, parent.Project.ID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.AssignedToID = userID
	}

	if req.CustomFields != nil {
		resolved, err := resolveCustomFieldsAPI(req.CustomFields, s.rules)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.CustomFields = resolved
	}

	issue, err := client.CreateIssue(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, formatIssueAPI(*issue))
}

// @Summary Add watcher
// @Description Add a watcher to an issue
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Issue ID"
// @Param request body object true "Watcher data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/watchers [post]
func (s *Server) handleAddWatcher(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	idStr := chi.URLParam(r, "id")
	issueID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	var req struct {
		User string `json:"user"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.User == "" {
		writeError(w, http.StatusBadRequest, "user is required")
		return
	}

	// Get issue for project context
	issue, err := client.GetIssue(issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	userID, err := resolver.ResolveUser(req.User, issue.Project.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := client.AddWatcher(issueID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"issue_id": issueID,
		"user_id":  userID,
		"message":  "Watcher added successfully",
	})
}

// @Summary Add relation
// @Description Create a relation between two issues
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Source Issue ID"
// @Param request body object true "Relation data"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/relations [post]
func (s *Server) handleAddRelation(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	issueID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	var req struct {
		IssueToID    int    `json:"issue_to_id"`
		RelationType string `json:"relation_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IssueToID == 0 || req.RelationType == "" {
		writeError(w, http.StatusBadRequest, "issue_to_id and relation_type are required")
		return
	}

	relation, err := client.CreateRelation(issueID, req.IssueToID, req.RelationType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            relation.ID,
		"issue_id":      relation.IssueID,
		"issue_to_id":   relation.IssueToID,
		"relation_type": relation.RelationType,
	})
}

// @Summary Create time entry
// @Description Log time on an issue
// @Tags Time Entries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body object true "Time entry data"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /time_entries [post]
func (s *Server) handleCreateTimeEntry(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	var req struct {
		IssueID  int     `json:"issue_id"`
		Hours    float64 `json:"hours"`
		Activity string  `json:"activity"`
		Comments string  `json:"comments"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IssueID == 0 || req.Hours == 0 {
		writeError(w, http.StatusBadRequest, "issue_id and hours are required")
		return
	}

	params := redmine.CreateTimeEntryParams{
		IssueID:  req.IssueID,
		Hours:    req.Hours,
		Comments: req.Comments,
	}

	if req.Activity != "" {
		activityID, err := resolver.ResolveActivity(req.Activity)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.ActivityID = activityID
	}

	entry, err := client.CreateTimeEntry(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       entry.ID,
		"issue_id": req.IssueID,
		"hours":    entry.Hours,
		"activity": entry.Activity.Name,
		"comments": entry.Comments,
		"spent_on": entry.SpentOn,
	})
}

// @Summary List trackers
// @Description Returns all available trackers
// @Tags Reference
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Router /trackers [get]
func (s *Server) handleListTrackers(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	trackers, err := client.ListTrackers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(trackers))
	for i, t := range trackers {
		result[i] = map[string]any{
			"id":   t.ID,
			"name": t.Name,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trackers": result,
		"count":    len(trackers),
	})
}

// @Summary List statuses
// @Description Returns all available issue statuses
// @Tags Reference
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Router /statuses [get]
func (s *Server) handleListStatuses(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	statuses, err := client.ListIssueStatuses()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(statuses))
	for i, s := range statuses {
		result[i] = map[string]any{
			"id":        s.ID,
			"name":      s.Name,
			"is_closed": s.IsClosed,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"statuses": result,
		"count":    len(statuses),
	})
}

// @Summary List activities
// @Description Returns all available time entry activities
// @Tags Reference
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Router /activities [get]
func (s *Server) handleListActivities(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	activities, err := client.ListTimeEntryActivities()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(activities))
	for i, a := range activities {
		result[i] = map[string]any{
			"id":         a.ID,
			"name":       a.Name,
			"is_default": a.IsDefault,
			"active":     a.Active,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"activities": result,
		"count":      len(activities),
	})
}

// @Summary List all custom fields
// @Description Returns all custom field definitions (requires admin privileges)
// @Tags Custom Fields
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param type query string false "Filter by customized type: issue, project, user, time_entry, version, group"
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /custom_fields [get]
func (s *Server) handleListAllCustomFields(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	fields, err := client.ListAllCustomFields()
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"Requires admin privileges. Use project-specific endpoints to see fields available in a specific project. Error: "+err.Error())
		return
	}

	typeFilter := r.URL.Query().Get("type")

	results := make([]map[string]any, 0)
	for _, f := range fields {
		if typeFilter != "" && f.CustomizedType != typeFilter {
			continue
		}
		field := map[string]any{
			"id":              f.ID,
			"name":            f.Name,
			"customized_type": f.CustomizedType,
			"field_format":    f.FieldFormat,
			"is_required":     f.IsRequired,
			"visible":         f.Visible,
		}
		if f.Multiple {
			field["multiple"] = true
		}
		if f.DefaultValue != "" {
			field["default_value"] = f.DefaultValue
		}
		if len(f.PossibleValues) > 0 {
			vals := make([]string, len(f.PossibleValues))
			for i, v := range f.PossibleValues {
				vals[i] = v.Value
			}
			field["possible_values"] = vals
		}
		if len(f.Trackers) > 0 {
			trackers := make([]map[string]any, len(f.Trackers))
			for i, t := range f.Trackers {
				trackers[i] = map[string]any{"id": t.ID, "name": t.Name}
			}
			field["trackers"] = trackers
		}
		results = append(results, field)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"custom_fields": results,
		"count":         len(results),
	})
}

// @Summary Get project details
// @Description Get project with enabled trackers and custom fields
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects/{id} [get]
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		// Try resolving by name/identifier
		resolver := redmine.NewResolver(client)
		id, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	project, err := client.GetProjectDetail(id, []string{"trackers", "issue_custom_fields"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := map[string]any{
		"id":          project.ID,
		"name":        project.Name,
		"identifier":  project.Identifier,
		"description": project.Description,
		"status":      project.Status,
	}

	if project.Parent != nil {
		result["parent"] = map[string]any{
			"id":   project.Parent.ID,
			"name": project.Parent.Name,
		}
	}

	trackers := make([]map[string]any, len(project.Trackers))
	for i, t := range project.Trackers {
		trackers[i] = map[string]any{"id": t.ID, "name": t.Name}
	}
	result["trackers"] = trackers

	customFields := make([]map[string]any, len(project.IssueCustomFields))
	for i, cf := range project.IssueCustomFields {
		customFields[i] = map[string]any{"id": cf.ID, "name": cf.Name}
	}
	result["issue_custom_fields"] = customFields

	writeJSON(w, http.StatusOK, result)
}

// @Summary Update project
// @Description Update project settings (trackers, custom fields, name, description)
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Param request body object true "Project update data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /projects/{id} [patch]
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		id, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	var req struct {
		Name                string   `json:"name"`
		Description         string   `json:"description"`
		TrackerIDs          []string `json:"tracker_ids"`
		IssueCustomFieldIDs []string `json:"issue_custom_field_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	params := redmine.UpdateProjectParams{
		ProjectID:   id,
		Name:        req.Name,
		Description: req.Description,
	}

	// Resolve tracker IDs (names or numeric IDs)
	if req.TrackerIDs != nil {
		trackerIDs := make([]int, 0, len(req.TrackerIDs))
		for _, s := range req.TrackerIDs {
			tid, err := resolver.ResolveTracker(s)
			if err != nil {
				writeError(w, http.StatusBadRequest, "Failed to resolve tracker '"+s+"': "+err.Error())
				return
			}
			trackerIDs = append(trackerIDs, tid)
		}
		params.TrackerIDs = trackerIDs
	}

	// Resolve custom field IDs (names or numeric IDs)
	if req.IssueCustomFieldIDs != nil {
		cfIDs := make([]int, 0, len(req.IssueCustomFieldIDs))
		for _, s := range req.IssueCustomFieldIDs {
			cfid, err := resolver.ResolveCustomFieldByName(s, id, 0)
			if err != nil {
				writeError(w, http.StatusBadRequest, "Failed to resolve custom field '"+s+"': "+err.Error())
				return
			}
			cfIDs = append(cfIDs, cfid)
		}
		params.IssueCustomFieldIDs = cfIDs
	}

	if err := client.UpdateProject(params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return updated project detail
	project, err := client.GetProjectDetail(id, []string{"trackers", "issue_custom_fields"})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":    true,
			"project_id": id,
			"message":    "Project updated successfully",
		})
		return
	}

	result := map[string]any{
		"success":    true,
		"project_id": project.ID,
		"name":       project.Name,
		"message":    "Project updated successfully",
	}

	trackers := make([]map[string]any, len(project.Trackers))
	for i, t := range project.Trackers {
		trackers[i] = map[string]any{"id": t.ID, "name": t.Name}
	}
	result["trackers"] = trackers

	customFields := make([]map[string]any, len(project.IssueCustomFields))
	for i, cf := range project.IssueCustomFields {
		customFields[i] = map[string]any{"id": cf.ID, "name": cf.Name}
	}
	result["issue_custom_fields"] = customFields

	writeJSON(w, http.StatusOK, result)
}

// Helper functions

func formatIssueAPI(issue redmine.Issue) map[string]any {
	result := map[string]any{
		"id":      issue.ID,
		"subject": issue.Subject,
		"project": map[string]any{
			"id":   issue.Project.ID,
			"name": issue.Project.Name,
		},
		"tracker": map[string]any{
			"id":   issue.Tracker.ID,
			"name": issue.Tracker.Name,
		},
		"status": map[string]any{
			"id":   issue.Status.ID,
			"name": issue.Status.Name,
		},
		"priority": map[string]any{
			"id":   issue.Priority.ID,
			"name": issue.Priority.Name,
		},
		"author": map[string]any{
			"id":   issue.Author.ID,
			"name": issue.Author.Name,
		},
		"created_on": issue.CreatedOn,
		"updated_on": issue.UpdatedOn,
	}

	if issue.AssignedTo != nil {
		result["assigned_to"] = map[string]any{
			"id":   issue.AssignedTo.ID,
			"name": issue.AssignedTo.Name,
		}
	}

	return result
}

func formatIssueDetailAPI(issue redmine.Issue) map[string]any {
	result := formatIssueAPI(issue)
	result["description"] = issue.Description
	result["done_ratio"] = issue.DoneRatio

	if issue.Parent != nil {
		result["parent_issue_id"] = issue.Parent.ID
	}

	if len(issue.CustomFields) > 0 {
		cf := make(map[string]any)
		for _, f := range issue.CustomFields {
			cf[f.Name] = f.Value
		}
		result["custom_fields"] = cf
	}

	if len(issue.Journals) > 0 {
		journals := make([]map[string]any, len(issue.Journals))
		for i, j := range issue.Journals {
			journals[i] = map[string]any{
				"id":         j.ID,
				"user":       j.User.Name,
				"notes":      j.Notes,
				"created_on": j.CreatedOn,
			}
		}
		result["journals"] = journals
	}

	if len(issue.AllowedStatuses) > 0 {
		statuses := make([]map[string]any, len(issue.AllowedStatuses))
		for i, s := range issue.AllowedStatuses {
			statuses[i] = map[string]any{
				"id":   s.ID,
				"name": s.Name,
			}
		}
		result["allowed_statuses"] = statuses
	}

	if len(issue.Attachments) > 0 {
		attachments := make([]map[string]any, len(issue.Attachments))
		for i, a := range issue.Attachments {
			attachments[i] = map[string]any{
				"id":           a.ID,
				"filename":     a.Filename,
				"filesize":     a.Filesize,
				"content_type": a.ContentType,
				"description":  a.Description,
				"created_on":   a.CreatedOn,
				"author":       map[string]any{"id": a.Author.ID, "name": a.Author.Name},
			}
		}
		result["attachments"] = attachments
	}

	return result
}

func resolveCustomFieldsAPI(fields map[string]any, rules *redmine.CustomFieldRules) (map[string]any, error) {
	result := make(map[string]any)

	// Build name-to-ID mapping from rules
	nameToID := make(map[string]int)
	if rules != nil {
		for idStr, field := range rules.Fields {
			if id, err := strconv.Atoi(idStr); err == nil {
				nameToID[strings.ToLower(field.Name)] = id
			}
		}
	}

	for name, value := range fields {
		var fieldID int

		if id, err := strconv.Atoi(name); err == nil {
			// Already a numeric ID
			fieldID = id
		} else if id, ok := nameToID[strings.ToLower(name)]; ok {
			// Resolve name to ID using rules
			fieldID = id
		} else {
			// Unknown field name, skip it (Redmine will reject invalid names anyway)
			continue
		}

		// Validate value if rules exist
		if rules != nil {
			if s, ok := value.(string); ok {
				corrected, verr := rules.ValidateValue(fieldID, s)
				if verr != nil {
					return nil, verr
				}
				value = corrected
			}
		}
		result[strconv.Itoa(fieldID)] = value
	}
	return result, nil
}

const maxUploadSize = 5 * 1024 * 1024   // 5MB per file
const maxMultipartMem = 10 * 1024 * 1024 // 10MB for multipart form

// @Summary Upload attachment
// @Description Upload a file to Redmine and get an upload token
// @Tags Attachments
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "File to upload"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /attachments/upload [post]
func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	if err := r.ParseMultipartForm(maxMultipartMem); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing 'file' field in multipart form")
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > maxUploadSize {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("File too large: %d bytes (max %d bytes / 5MB)", header.Size, maxUploadSize))
		return
	}

	token, err := client.UploadFile(header.Filename, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to upload file: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":    token.Token,
		"filename": token.Filename,
		"size":     header.Size,
	})
}

// @Summary Download attachment
// @Description Download an attachment by ID
// @Tags Attachments
// @Produce octet-stream
// @Security ApiKeyAuth
// @Param id path int true "Attachment ID"
// @Success 200 {file} binary
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /attachments/{id}/download [get]
func (s *Server) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid attachment ID")
		return
	}

	data, contentType, filename, err := client.DownloadAttachment(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to download attachment: "+err.Error())
		return
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// @Summary List issue attachments
// @Description List attachments on an issue
// @Tags Attachments
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Issue ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/attachments [get]
func (s *Server) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	issue, err := client.GetIssue(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get issue: "+err.Error())
		return
	}

	attachments := make([]map[string]any, len(issue.Attachments))
	for i, a := range issue.Attachments {
		attachments[i] = map[string]any{
			"id":           a.ID,
			"filename":     a.Filename,
			"filesize":     a.Filesize,
			"content_type": a.ContentType,
			"description":  a.Description,
			"created_on":   a.CreatedOn,
			"author":       map[string]any{"id": a.Author.ID, "name": a.Author.Name},
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"issue_id":    id,
		"attachments": attachments,
		"count":       len(attachments),
	})
}

// @Summary Attach files to issue
// @Description Upload one or more files and attach them to an issue
// @Tags Attachments
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Issue ID"
// @Param files[] formData file true "Files to attach"
// @Param notes formData string false "Notes/comment to add"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/attach [post]
func (s *Server) handleAttachToIssue(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	issueID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	if err := r.ParseMultipartForm(maxMultipartMem); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	// Support both "files[]" and "file" field names
	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "No files provided. Use 'files[]' or 'file' field name.")
		return
	}

	// Upload each file and collect tokens
	var uploads []redmine.UploadToken
	for _, fileHeader := range files {
		if fileHeader.Size > maxUploadSize {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("File '%s' too large: %d bytes (max %d bytes / 5MB)", fileHeader.Filename, fileHeader.Size, maxUploadSize))
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to open uploaded file: "+err.Error())
			return
		}

		content, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to read uploaded file: "+err.Error())
			return
		}

		token, err := client.UploadFile(fileHeader.Filename, io.NopCloser(io.Reader(bytes.NewReader(content))))
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to upload '%s': %v", fileHeader.Filename, err))
			return
		}
		token.ContentType = fileHeader.Header.Get("Content-Type")
		uploads = append(uploads, *token)
	}

	// Attach all files to issue
	notes := r.FormValue("notes")
	params := redmine.UpdateIssueParams{
		IssueID: issueID,
		Notes:   notes,
		Uploads: uploads,
	}

	if err := client.UpdateIssue(params); err != nil {
		writeError(w, http.StatusInternalServerError, "Files uploaded but failed to attach: "+err.Error())
		return
	}

	attached := make([]map[string]any, len(uploads))
	for i, u := range uploads {
		attached[i] = map[string]any{
			"filename": u.Filename,
			"token":    u.Token,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"issue_id": issueID,
		"attached": attached,
		"count":    len(uploads),
		"message":  "Files attached to issue successfully",
	})
}

// --- Group A: CRUD Gaps ---

// @Summary Update time entry
// @Description Update an existing time entry
// @Tags Time Entries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Time Entry ID"
// @Param request body object true "Time entry update data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /time_entries/{id} [patch]
func (s *Server) handleUpdateTimeEntry(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid time entry ID")
		return
	}

	var req struct {
		Hours    float64 `json:"hours"`
		Activity string  `json:"activity"`
		Comments string  `json:"comments"`
		SpentOn  string  `json:"spent_on"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	params := redmine.UpdateTimeEntryParams{
		TimeEntryID: id,
		Hours:       req.Hours,
		Comments:    req.Comments,
		SpentOn:     req.SpentOn,
	}

	if req.Activity != "" {
		activityID, err := resolver.ResolveActivity(req.Activity)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.ActivityID = activityID
	}

	if err := client.UpdateTimeEntry(params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"time_entry_id": id,
		"message":       "Time entry updated successfully",
	})
}

// @Summary Delete time entry
// @Description Delete a time entry
// @Tags Time Entries
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Time Entry ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /time_entries/{id} [delete]
func (s *Server) handleDeleteTimeEntry(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid time entry ID")
		return
	}

	if err := client.DeleteTimeEntry(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"time_entry_id": id,
		"message":       "Time entry deleted successfully",
	})
}

// @Summary Remove watcher
// @Description Remove a watcher from an issue
// @Tags Issues
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Issue ID"
// @Param user_id path int true "User ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/watchers/{user_id} [delete]
func (s *Server) handleRemoveWatcher(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	issueID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	userIDStr := chi.URLParam(r, "user_id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := client.RemoveWatcher(issueID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"issue_id": issueID,
		"user_id":  userID,
		"message":  "Watcher removed successfully",
	})
}

// @Summary Delete relation
// @Description Delete an issue relation
// @Tags Issues
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Relation ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /relations/{id} [delete]
func (s *Server) handleDeleteRelation(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid relation ID")
		return
	}

	if err := client.DeleteRelation(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"relation_id": id,
		"message":     "Relation deleted successfully",
	})
}

// --- Group E: User Search ---

// @Summary Search users
// @Description Search for users by name, optionally within a project
// @Tags Users
// @Produce json
// @Security ApiKeyAuth
// @Param name query string false "User name filter"
// @Param project query string false "Project name or ID for context"
// @Param status query int false "User status: 1=active, 2=registered, 3=locked"
// @Param limit query int false "Number of results" default(25)
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /users [get]
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	q := r.URL.Query()
	params := redmine.SearchUsersParams{
		Name: q.Get("name"),
	}

	if project := q.Get("project"); project != "" {
		projectID, err := resolver.ResolveProject(project)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.ProjectID = projectID
	}

	if status := q.Get("status"); status != "" {
		if n, err := strconv.Atoi(status); err == nil {
			params.Status = n
		}
	}

	if limit := q.Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			params.Limit = n
		}
	}

	users, total, err := client.SearchUsers(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(users))
	for i, u := range users {
		result[i] = map[string]any{
			"id":   u.ID,
			"name": u.Name,
		}
		if u.Login != "" {
			result[i]["login"] = u.Login
		}
		if u.Mail != "" {
			result[i]["email"] = u.Mail
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"users":       result,
		"count":       len(users),
		"total_count": total,
	})
}

// --- Group B: Batch & Copy ---

// @Summary Batch update issues
// @Description Update multiple issues at once
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body object true "Batch update data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/batch-update [post]
func (s *Server) handleBatchUpdateIssues(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	var req struct {
		IssueIDs   []int  `json:"issue_ids"`
		Status     string `json:"status"`
		AssignedTo string `json:"assigned_to"`
		Priority   string `json:"priority"`
		Notes      string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.IssueIDs) == 0 {
		writeError(w, http.StatusBadRequest, "issue_ids is required and must not be empty")
		return
	}

	// Pre-resolve names to IDs once
	var statusID, priorityID, assignedToID int

	if req.Status != "" {
		var err error
		statusID, err = resolver.ResolveStatusID(req.Status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.Priority != "" {
		var err error
		priorityID, err = resolver.ResolvePriority(req.Priority)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// For assigned_to, we may need project context per issue, so resolve per-issue below
	// unless it's a numeric ID
	if req.AssignedTo != "" {
		if id, err := strconv.Atoi(req.AssignedTo); err == nil {
			assignedToID = id
		}
	}

	var successIDs []int
	var failures []map[string]any

	for _, issueID := range req.IssueIDs {
		params := redmine.UpdateIssueParams{
			IssueID:    issueID,
			StatusID:   statusID,
			PriorityID: priorityID,
			Notes:      req.Notes,
		}

		if assignedToID > 0 {
			params.AssignedToID = assignedToID
		} else if req.AssignedTo != "" {
			// Need to resolve user with project context
			issue, err := client.GetIssue(issueID)
			if err != nil {
				failures = append(failures, map[string]any{"id": issueID, "error": err.Error()})
				continue
			}
			uid, err := resolver.ResolveUser(req.AssignedTo, issue.Project.ID)
			if err != nil {
				failures = append(failures, map[string]any{"id": issueID, "error": err.Error()})
				continue
			}
			params.AssignedToID = uid
		}

		if err := client.UpdateIssue(params); err != nil {
			failures = append(failures, map[string]any{"id": issueID, "error": err.Error()})
			continue
		}
		successIDs = append(successIDs, issueID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": successIDs,
		"failed":  failures,
	})
}

// @Summary Copy issue
// @Description Copy an issue, optionally to a different project with a new subject
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Source Issue ID"
// @Param request body object false "Copy options"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/{id}/copy [post]
func (s *Server) handleCopyIssue(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	idStr := chi.URLParam(r, "id")
	sourceID, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid issue ID")
		return
	}

	var req struct {
		Project string `json:"project"`
		Subject string `json:"subject"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get source issue
	source, err := client.GetIssue(sourceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build create params from source
	params := redmine.CreateIssueParams{
		ProjectID:   source.Project.ID,
		TrackerID:   source.Tracker.ID,
		Subject:     source.Subject,
		Description: source.Description,
	}

	if source.AssignedTo != nil {
		params.AssignedToID = source.AssignedTo.ID
	}

	// Override project if specified
	if req.Project != "" {
		projectID, err := resolver.ResolveProject(req.Project)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.ProjectID = projectID
	}

	// Override subject if specified
	if req.Subject != "" {
		params.Subject = req.Subject
	}

	// Copy custom fields
	if len(source.CustomFields) > 0 {
		cf := make(map[string]any)
		for _, f := range source.CustomFields {
			cf[strconv.Itoa(f.ID)] = f.Value
		}
		params.CustomFields = cf
	}

	issue, err := client.CreateIssue(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, formatIssueAPI(*issue))
}

// --- Group C: Versions ---

// @Summary List versions
// @Description List all versions for a project
// @Tags Versions
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects/{id}/versions [get]
func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	projectID, err := strconv.Atoi(idStr)
	if err != nil {
		// Try resolving by name
		resolver := redmine.NewResolver(client)
		projectID, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	versions, err := client.ListVersions(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(versions))
	for i, v := range versions {
		result[i] = map[string]any{
			"id":          v.ID,
			"name":        v.Name,
			"description": v.Description,
			"status":      v.Status,
			"due_date":    v.DueDate,
			"sharing":     v.Sharing,
			"created_on":  v.CreatedOn,
			"updated_on":  v.UpdatedOn,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"versions": result,
		"count":    len(versions),
	})
}

// @Summary Create version
// @Description Create a new version in a project
// @Tags Versions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Param request body object true "Version data"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects/{id}/versions [post]
func (s *Server) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	projectID, err := strconv.Atoi(idStr)
	if err != nil {
		resolver := redmine.NewResolver(client)
		projectID, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Status      string `json:"status"`
		DueDate     string `json:"due_date"`
		Sharing     string `json:"sharing"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	params := redmine.CreateVersionParams{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		DueDate:     req.DueDate,
		Sharing:     req.Sharing,
	}

	version, err := client.CreateVersion(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          version.ID,
		"name":        version.Name,
		"description": version.Description,
		"status":      version.Status,
		"due_date":    version.DueDate,
		"sharing":     version.Sharing,
	})
}

// @Summary Update version
// @Description Update an existing version
// @Tags Versions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Version ID"
// @Param request body object true "Version update data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /versions/{id} [patch]
func (s *Server) handleUpdateVersion(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid version ID")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Status      string `json:"status"`
		DueDate     string `json:"due_date"`
		Sharing     string `json:"sharing"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	params := redmine.UpdateVersionParams{
		VersionID:   id,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		DueDate:     req.DueDate,
		Sharing:     req.Sharing,
	}

	if err := client.UpdateVersion(params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"version_id": id,
		"message":    "Version updated successfully",
	})
}

// --- Group D: Wiki ---

// @Summary List wiki pages
// @Description List all wiki pages in a project
// @Tags Wiki
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects/{id}/wiki [get]
func (s *Server) handleListWikiPages(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	projectID, err := strconv.Atoi(idStr)
	if err != nil {
		resolver := redmine.NewResolver(client)
		projectID, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	pages, err := client.ListWikiPages(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]any, len(pages))
	for i, p := range pages {
		result[i] = map[string]any{
			"title":      p.Title,
			"version":    p.Version,
			"created_on": p.CreatedOn,
			"updated_on": p.UpdatedOn,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"wiki_pages": result,
		"count":      len(pages),
	})
}

// @Summary Get wiki page
// @Description Get a wiki page with content
// @Tags Wiki
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Param title path string true "Wiki page title"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects/{id}/wiki/{title} [get]
func (s *Server) handleGetWikiPage(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	projectID, err := strconv.Atoi(idStr)
	if err != nil {
		resolver := redmine.NewResolver(client)
		projectID, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	title := chi.URLParam(r, "title")
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	page, err := client.GetWikiPage(projectID, title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"title":      page.Title,
		"text":       page.Text,
		"version":    page.Version,
		"author":     map[string]any{"id": page.Author.ID, "name": page.Author.Name},
		"comments":   page.Comments,
		"created_on": page.CreatedOn,
		"updated_on": page.UpdatedOn,
	})
}

// @Summary Create or update wiki page
// @Description Create a new wiki page or update an existing one
// @Tags Wiki
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "Project ID"
// @Param title path string true "Wiki page title"
// @Param request body object true "Wiki page data"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /projects/{id}/wiki/{title} [put]
func (s *Server) handleCreateOrUpdateWikiPage(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	idStr := chi.URLParam(r, "id")
	projectID, err := strconv.Atoi(idStr)
	if err != nil {
		resolver := redmine.NewResolver(client)
		projectID, err = resolver.ResolveProject(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid project ID or name: "+err.Error())
			return
		}
	}

	title := chi.URLParam(r, "title")
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	var req struct {
		Text     string `json:"text"`
		Comments string `json:"comments"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	params := redmine.WikiPageParams{
		ProjectID: projectID,
		Title:     title,
		Text:      req.Text,
		Comments:  req.Comments,
	}

	if err := client.CreateOrUpdateWikiPage(params); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"project_id": projectID,
		"title":      title,
		"message":    "Wiki page saved successfully",
	})
}

// --- Group F: Export ---

// @Summary Export issues as CSV
// @Description Export issues matching filters as a CSV file
// @Tags Issues
// @Produce text/csv
// @Security ApiKeyAuth
// @Param project query string false "Project name or ID"
// @Param tracker query string false "Tracker name or ID"
// @Param status query string false "Status: open, closed, all, or specific name"
// @Param assigned_to query string false "Assignee name or 'me'"
// @Param limit query int false "Number of issues to export" default(100)
// @Success 200 {file} binary
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /issues/export.csv [get]
func (s *Server) handleExportIssuesCSV(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())
	resolver := redmine.NewResolver(client)

	params := redmine.SearchIssuesParams{}
	q := r.URL.Query()

	if project := q.Get("project"); project != "" {
		projectID, err := resolver.ResolveProject(project)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.ProjectID = strconv.Itoa(projectID)
	}

	if tracker := q.Get("tracker"); tracker != "" {
		trackerID, err := resolver.ResolveTracker(tracker)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.TrackerID = trackerID
	}

	if status := q.Get("status"); status != "" {
		statusID, err := resolver.ResolveStatus(status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.StatusID = statusID
	} else {
		params.StatusID = "open"
	}

	if assignedTo := q.Get("assigned_to"); assignedTo != "" {
		if assignedTo == "me" {
			params.AssignedToID = "me"
		} else {
			var projectID int
			if params.ProjectID != "" {
				projectID, _ = strconv.Atoi(params.ProjectID)
			}
			userID, err := resolver.ResolveUser(assignedTo, projectID)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			params.AssignedToID = strconv.Itoa(userID)
		}
	}

	if limit := q.Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			params.Limit = n
		}
	} else {
		params.Limit = 100
	}

	params.Sort = q.Get("sort")

	issues, _, err := client.SearchIssues(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build CSV
	var buf bytes.Buffer
	csvWriter := csv.NewWriter(&buf)

	// Header
	_ = csvWriter.Write([]string{"ID", "Subject", "Project", "Tracker", "Status", "Priority", "Assignee", "Created", "Updated"})

	for _, issue := range issues {
		assignee := ""
		if issue.AssignedTo != nil {
			assignee = issue.AssignedTo.Name
		}
		_ = csvWriter.Write([]string{
			strconv.Itoa(issue.ID),
			issue.Subject,
			issue.Project.Name,
			issue.Tracker.Name,
			issue.Status.Name,
			issue.Priority.Name,
			assignee,
			issue.CreatedOn,
			issue.UpdatedOn,
		})
	}

	csvWriter.Flush()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=issues.csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// --- Group G: Reports ---

// @Summary Weekly report
// @Description Generate a weekly time report for a user
// @Tags Reports
// @Produce json
// @Security ApiKeyAuth
// @Param user query string false "User name or 'me'" default(me)
// @Param week_of query string false "Date within the week (YYYY-MM-DD), defaults to current week"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /reports/weekly [get]
func (s *Server) handleWeeklyReport(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	q := r.URL.Query()

	// Determine user
	userParam := q.Get("user")
	if userParam == "" {
		userParam = "me"
	}

	var userID string
	if userParam == "me" {
		userID = "me"
	} else {
		if id, err := strconv.Atoi(userParam); err == nil {
			userID = strconv.Itoa(id)
		} else {
			// Try to resolve user name -- need a project context or admin API
			// For reports, use "me" as fallback
			userID = "me"
		}
	}

	// Determine week boundaries (Monday to Friday)
	now := time.Now()
	weekOf := q.Get("week_of")
	var refDate time.Time
	if weekOf != "" {
		parsed, err := time.Parse("2006-01-02", weekOf)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid week_of date format, expected YYYY-MM-DD")
			return
		}
		refDate = parsed
	} else {
		refDate = now
	}

	// Calculate Monday of the week
	weekday := refDate.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	monday := refDate.AddDate(0, 0, -int(weekday-time.Monday))
	friday := monday.AddDate(0, 0, 4)

	mondayStr := monday.Format("2006-01-02")
	fridayStr := friday.Format("2006-01-02")

	// Fetch time entries for the week
	teParams := redmine.ListTimeEntriesParams{
		UserID: userID,
		From:   mondayStr,
		To:     fridayStr,
		Limit:  100,
	}

	entries, _, err := client.ListTimeEntries(teParams)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Aggregate by day and by project/issue
	dailyTotals := make(map[string]float64)
	projectTotals := make(map[string]float64)
	var totalHours float64
	entryDetails := make([]map[string]any, len(entries))

	for i, e := range entries {
		dailyTotals[e.SpentOn] += e.Hours
		projectTotals[e.Project.Name] += e.Hours
		totalHours += e.Hours

		detail := map[string]any{
			"id":       e.ID,
			"project":  e.Project.Name,
			"hours":    e.Hours,
			"activity": e.Activity.Name,
			"comments": e.Comments,
			"spent_on": e.SpentOn,
		}
		if e.Issue != nil {
			detail["issue_id"] = e.Issue.ID
		}
		entryDetails[i] = detail
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"week":           mondayStr + " to " + fridayStr,
		"user":           userParam,
		"total_hours":    totalHours,
		"daily_totals":   dailyTotals,
		"project_totals": projectTotals,
		"entries":        entryDetails,
		"entry_count":    len(entries),
	})
}

// @Summary Standup report
// @Description Generate a standup report showing yesterday's work and today's open issues
// @Tags Reports
// @Produce json
// @Security ApiKeyAuth
// @Param user query string false "User name or 'me'" default(me)
// @Param date query string false "Date for the report (YYYY-MM-DD), defaults to today"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /reports/standup [get]
func (s *Server) handleStandupReport(w http.ResponseWriter, r *http.Request) {
	client := getClient(r.Context())

	q := r.URL.Query()

	// Determine user
	userParam := q.Get("user")
	if userParam == "" {
		userParam = "me"
	}

	// Determine date
	now := time.Now()
	dateStr := q.Get("date")
	var today time.Time
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD")
			return
		}
		today = parsed
	} else {
		today = now
	}

	// Calculate yesterday (skip weekends)
	yesterday := today.AddDate(0, 0, -1)
	if yesterday.Weekday() == time.Sunday {
		yesterday = yesterday.AddDate(0, 0, -2) // Go to Friday
	} else if yesterday.Weekday() == time.Saturday {
		yesterday = yesterday.AddDate(0, 0, -1) // Go to Friday
	}

	todayStr := today.Format("2006-01-02")
	yesterdayStr := yesterday.Format("2006-01-02")

	// Fetch yesterday's time entries
	teParams := redmine.ListTimeEntriesParams{
		UserID: userParam,
		From:   yesterdayStr,
		To:     yesterdayStr,
		Limit:  100,
	}

	yesterdayEntries, _, err := client.ListTimeEntries(teParams)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var yesterdayHours float64
	yesterdayWork := make([]map[string]any, len(yesterdayEntries))
	for i, e := range yesterdayEntries {
		yesterdayHours += e.Hours
		work := map[string]any{
			"project":  e.Project.Name,
			"hours":    e.Hours,
			"activity": e.Activity.Name,
			"comments": e.Comments,
		}
		if e.Issue != nil {
			work["issue_id"] = e.Issue.ID
		}
		yesterdayWork[i] = work
	}

	// Fetch today's open issues assigned to user
	issueParams := redmine.SearchIssuesParams{
		AssignedToID: userParam,
		StatusID:     "open",
		Limit:        25,
		Sort:         "priority:desc,updated_on:desc",
	}

	todayIssues, _, err := client.SearchIssues(issueParams)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	openIssues := make([]map[string]any, len(todayIssues))
	for i, issue := range todayIssues {
		openIssues[i] = map[string]any{
			"id":       issue.ID,
			"subject":  issue.Subject,
			"project":  issue.Project.Name,
			"tracker":  issue.Tracker.Name,
			"status":   issue.Status.Name,
			"priority": issue.Priority.Name,
		}
		if issue.AssignedTo != nil {
			openIssues[i]["assigned_to"] = issue.AssignedTo.Name
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"date": todayStr,
		"user": userParam,
		"yesterday": map[string]any{
			"date":        yesterdayStr,
			"total_hours": yesterdayHours,
			"work":        yesterdayWork,
			"entry_count": len(yesterdayEntries),
		},
		"today": map[string]any{
			"open_issues": openIssues,
			"issue_count": len(todayIssues),
		},
	})
}

