package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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

	return result
}

func resolveCustomFieldsAPI(fields map[string]any, rules *redmine.CustomFieldRules) (map[string]any, error) {
	result := make(map[string]any)
	for name, value := range fields {
		if id, err := strconv.Atoi(name); err == nil {
			// Validate value if rules exist
			if rules != nil {
				if s, ok := value.(string); ok {
					corrected, verr := rules.ValidateValue(id, s)
					if verr != nil {
						return nil, verr
					}
					value = corrected
				}
			}
			result[strconv.Itoa(id)] = value
		} else {
			result[name] = value
		}
	}
	return result, nil
}
