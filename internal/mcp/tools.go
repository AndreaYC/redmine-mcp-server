package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

// ToolHandlers contains all MCP tool handlers
type ToolHandlers struct {
	client   *redmine.Client
	resolver *redmine.Resolver
}

// NewToolHandlers creates new tool handlers
func NewToolHandlers(client *redmine.Client) *ToolHandlers {
	return &ToolHandlers{
		client:   client,
		resolver: redmine.NewResolver(client),
	}
}

// RegisterTools registers all MCP tools on the server
func (h *ToolHandlers) RegisterTools(s McpServer) {
	// Account
	s.AddTool(mcp.NewTool("me",
		mcp.WithDescription("Get current user information"),
	), h.handleMe)

	// Projects
	s.AddTool(mcp.NewTool("projects.list",
		mcp.WithDescription("List all projects"),
		mcp.WithNumber("limit",
			mcp.Description("Number of projects to return (default: 100)"),
		),
	), h.handleProjectsList)

	s.AddTool(mcp.NewTool("projects.create",
		mcp.WithDescription("Create a new project"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Project name"),
		),
		mcp.WithString("identifier",
			mcp.Required(),
			mcp.Description("Project identifier (used in URLs)"),
		),
		mcp.WithString("description",
			mcp.Description("Project description"),
		),
		mcp.WithString("parent",
			mcp.Description("Parent project name or ID"),
		),
	), h.handleProjectsCreate)

	// Issues
	s.AddTool(mcp.NewTool("issues.search",
		mcp.WithDescription("Search issues"),
		mcp.WithString("project",
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("tracker",
			mcp.Description("Tracker name or ID"),
		),
		mcp.WithString("status",
			mcp.Description("Status: open, closed, all, or specific status name"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("Assignee name or 'me' for current user"),
		),
		mcp.WithString("subject",
			mcp.Description("Search keyword in issue subject (partial match)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of issues to return (default: 25)"),
		),
	), h.handleIssuesSearch)

	s.AddTool(mcp.NewTool("issues.getById",
		mcp.WithDescription("Get issue details including journals, watchers, and relations"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
	), h.handleIssuesGetById)

	s.AddTool(mcp.NewTool("issues.create",
		mcp.WithDescription("Create a new issue"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("tracker",
			mcp.Required(),
			mcp.Description("Tracker name or ID"),
		),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("Issue subject/title"),
		),
		mcp.WithString("description",
			mcp.Description("Issue description"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("Assignee name or ID"),
		),
		mcp.WithNumber("parent_issue_id",
			mcp.Description("Parent issue ID"),
		),
		mcp.WithString("start_date",
			mcp.Description("Start date (YYYY-MM-DD)"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date (YYYY-MM-DD)"),
		),
		mcp.WithObject("custom_fields",
			mcp.Description("Custom fields as key-value pairs (field name -> value)"),
		),
	), h.handleIssuesCreate)

	s.AddTool(mcp.NewTool("issues.update",
		mcp.WithDescription("Update an issue"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
		mcp.WithString("status",
			mcp.Description("New status name or ID"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("New assignee name or ID"),
		),
		mcp.WithString("notes",
			mcp.Description("Notes/comment to add"),
		),
		mcp.WithObject("custom_fields",
			mcp.Description("Custom fields to update as key-value pairs"),
		),
	), h.handleIssuesUpdate)

	s.AddTool(mcp.NewTool("issues.createSubtask",
		mcp.WithDescription("Create a subtask under an existing issue"),
		mcp.WithNumber("parent_issue_id",
			mcp.Required(),
			mcp.Description("Parent issue ID"),
		),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("Subtask subject/title"),
		),
		mcp.WithString("tracker",
			mcp.Description("Tracker name or ID (defaults to parent's tracker)"),
		),
		mcp.WithString("description",
			mcp.Description("Subtask description"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("Assignee name or ID"),
		),
		mcp.WithObject("custom_fields",
			mcp.Description("Custom fields as key-value pairs"),
		),
	), h.handleIssuesCreateSubtask)

	s.AddTool(mcp.NewTool("issues.addWatcher",
		mcp.WithDescription("Add a watcher to an issue"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("User name or ID to add as watcher"),
		),
	), h.handleIssuesAddWatcher)

	s.AddTool(mcp.NewTool("issues.addRelation",
		mcp.WithDescription("Create a relation between two issues"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Source issue ID"),
		),
		mcp.WithNumber("issue_to_id",
			mcp.Required(),
			mcp.Description("Target issue ID"),
		),
		mcp.WithString("relation_type",
			mcp.Required(),
			mcp.Description("Relation type: relates, duplicates, blocks, precedes, copied_to"),
			mcp.Enum("relates", "duplicates", "blocks", "precedes", "copied_to"),
		),
	), h.handleIssuesAddRelation)

	// Time Entries
	s.AddTool(mcp.NewTool("timeEntries.create",
		mcp.WithDescription("Create a time entry for an issue"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
		mcp.WithNumber("hours",
			mcp.Required(),
			mcp.Description("Hours spent"),
		),
		mcp.WithString("activity",
			mcp.Description("Activity name or ID"),
		),
		mcp.WithString("comments",
			mcp.Description("Comments"),
		),
	), h.handleTimeEntriesCreate)

	// Reference
	s.AddTool(mcp.NewTool("trackers.list",
		mcp.WithDescription("List all trackers"),
	), h.handleTrackersList)

	s.AddTool(mcp.NewTool("statuses.list",
		mcp.WithDescription("List all issue statuses"),
	), h.handleStatusesList)

	s.AddTool(mcp.NewTool("activities.list",
		mcp.WithDescription("List all time entry activities"),
	), h.handleActivitiesList)
}

// McpServer interface for registering tools
type McpServer interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}

// Handler implementations

func (h *ToolHandlers) handleMe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := h.client.GetCurrentUser()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get current user: %v", err)), nil
	}

	result := map[string]any{
		"id":        user.ID,
		"login":     user.Login,
		"firstname": user.Firstname,
		"lastname":  user.Lastname,
		"name":      user.Firstname + " " + user.Lastname,
		"email":     user.Mail,
	}

	return jsonResult(result)
}

func (h *ToolHandlers) handleProjectsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := req.GetInt("limit", 100)

	projects, err := h.client.ListProjects(limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list projects: %v", err)), nil
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

	return jsonResult(map[string]any{
		"projects": result,
		"count":    len(projects),
	})
}

func (h *ToolHandlers) handleProjectsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	identifier, err := req.RequireString("identifier")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	description := req.GetString("description", "")

	var parentID int
	if parent := req.GetString("parent", ""); parent != "" {
		parentID, err = h.resolver.ResolveProject(parent)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve parent project: %v", err)), nil
		}
	}

	project, err := h.client.CreateProject(name, identifier, description, parentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create project: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"id":         project.ID,
		"name":       project.Name,
		"identifier": project.Identifier,
	})
}

func (h *ToolHandlers) handleIssuesSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := redmine.SearchIssuesParams{}

	if project := req.GetString("project", ""); project != "" {
		projectID, err := h.resolver.ResolveProject(project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
		}
		params.ProjectID = strconv.Itoa(projectID)
	}

	if tracker := req.GetString("tracker", ""); tracker != "" {
		trackerID, err := h.resolver.ResolveTracker(tracker)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
		}
		params.TrackerID = trackerID
	}

	if status := req.GetString("status", ""); status != "" {
		statusID, err := h.resolver.ResolveStatus(status)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve status: %v", err)), nil
		}
		params.StatusID = statusID
	} else {
		params.StatusID = "open"
	}

	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		if assignedTo == "me" {
			params.AssignedToID = "me"
		} else {
			var projectID int
			if params.ProjectID != "" {
				projectID, _ = strconv.Atoi(params.ProjectID)
			}
			userID, err := h.resolver.ResolveUser(assignedTo, projectID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
			}
			params.AssignedToID = strconv.Itoa(userID)
		}
	}

	// Subject keyword search
	params.Subject = req.GetString("subject", "")

	params.Limit = req.GetInt("limit", 25)

	issues, total, err := h.client.SearchIssues(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search issues: %v", err)), nil
	}

	result := make([]map[string]any, len(issues))
	for i, issue := range issues {
		result[i] = formatIssue(issue)
	}

	return jsonResult(map[string]any{
		"issues":      result,
		"count":       len(issues),
		"total_count": total,
	})
}

func (h *ToolHandlers) handleIssuesGetById(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(issueIDFloat)

	issue, err := h.client.GetIssue(issueID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get issue: %v", err)), nil
	}

	result := formatIssueDetail(*issue)
	return jsonResult(result)
}

func (h *ToolHandlers) handleIssuesCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tracker, err := req.RequireString("tracker")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	subject, err := req.RequireString("subject")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(project)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	trackerID, err := h.resolver.ResolveTracker(tracker)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
	}

	params := redmine.CreateIssueParams{
		ProjectID: projectID,
		TrackerID: trackerID,
		Subject:   subject,
	}

	params.Description = req.GetString("description", "")

	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		userID, err := h.resolver.ResolveUser(assignedTo, projectID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve assignee: %v", err)), nil
		}
		params.AssignedToID = userID
	}

	params.ParentIssueID = req.GetInt("parent_issue_id", 0)
	params.StartDate = req.GetString("start_date", "")
	params.DueDate = req.GetString("due_date", "")

	if customFields := getMapArg(req, "custom_fields"); customFields != nil {
		params.CustomFields = h.resolveCustomFields(customFields, projectID)
	}

	issue, err := h.client.CreateIssue(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create issue: %v", err)), nil
	}

	return jsonResult(formatIssue(*issue))
}

func (h *ToolHandlers) handleIssuesUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(issueIDFloat)

	params := redmine.UpdateIssueParams{
		IssueID: issueID,
	}

	// Get issue to resolve project context for user lookup
	issue, err := h.client.GetIssue(issueID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get issue: %v", err)), nil
	}

	if status := req.GetString("status", ""); status != "" {
		statusID, err := h.resolver.ResolveStatusID(status)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve status: %v", err)), nil
		}
		params.StatusID = statusID
	}

	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		userID, err := h.resolver.ResolveUser(assignedTo, issue.Project.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve assignee: %v", err)), nil
		}
		params.AssignedToID = userID
	}

	params.Notes = req.GetString("notes", "")

	if customFields := getMapArg(req, "custom_fields"); customFields != nil {
		params.CustomFields = h.resolveCustomFields(customFields, issue.Project.ID)
	}

	if err := h.client.UpdateIssue(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update issue: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":  true,
		"issue_id": issueID,
		"message":  "Issue updated successfully",
	})
}

func (h *ToolHandlers) handleIssuesCreateSubtask(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	parentIDFloat, err := req.RequireFloat("parent_issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	parentID := int(parentIDFloat)

	subject, err := req.RequireString("subject")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get parent issue to get project and default tracker
	parent, err := h.client.GetIssue(parentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get parent issue: %v", err)), nil
	}

	params := redmine.CreateIssueParams{
		ProjectID:     parent.Project.ID,
		TrackerID:     parent.Tracker.ID,
		Subject:       subject,
		ParentIssueID: parentID,
	}

	if tracker := req.GetString("tracker", ""); tracker != "" {
		trackerID, err := h.resolver.ResolveTracker(tracker)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
		}
		params.TrackerID = trackerID
	}

	params.Description = req.GetString("description", "")

	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		userID, err := h.resolver.ResolveUser(assignedTo, parent.Project.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve assignee: %v", err)), nil
		}
		params.AssignedToID = userID
	}

	if customFields := getMapArg(req, "custom_fields"); customFields != nil {
		params.CustomFields = h.resolveCustomFields(customFields, parent.Project.ID)
	}

	issue, err := h.client.CreateIssue(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create subtask: %v", err)), nil
	}

	return jsonResult(formatIssue(*issue))
}

func (h *ToolHandlers) handleIssuesAddWatcher(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(issueIDFloat)

	user, err := req.RequireString("user")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get issue to resolve project context
	issue, err := h.client.GetIssue(issueID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get issue: %v", err)), nil
	}

	userID, err := h.resolver.ResolveUser(user, issue.Project.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
	}

	if err := h.client.AddWatcher(issueID, userID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to add watcher: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":  true,
		"issue_id": issueID,
		"user_id":  userID,
		"message":  "Watcher added successfully",
	})
}

func (h *ToolHandlers) handleIssuesAddRelation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(issueIDFloat)

	issueToIDFloat, err := req.RequireFloat("issue_to_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueToID := int(issueToIDFloat)

	relationType, err := req.RequireString("relation_type")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	relation, err := h.client.CreateRelation(issueID, issueToID, relationType)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create relation: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"id":            relation.ID,
		"issue_id":      relation.IssueID,
		"issue_to_id":   relation.IssueToID,
		"relation_type": relation.RelationType,
	})
}

func (h *ToolHandlers) handleTimeEntriesCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(issueIDFloat)

	hours, err := req.RequireFloat("hours")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	params := redmine.CreateTimeEntryParams{
		IssueID: issueID,
		Hours:   hours,
	}

	if activity := req.GetString("activity", ""); activity != "" {
		activityID, err := h.resolver.ResolveActivity(activity)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve activity: %v", err)), nil
		}
		params.ActivityID = activityID
	}

	params.Comments = req.GetString("comments", "")

	entry, err := h.client.CreateTimeEntry(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create time entry: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"id":       entry.ID,
		"issue_id": issueID,
		"hours":    entry.Hours,
		"activity": entry.Activity.Name,
		"comments": entry.Comments,
		"spent_on": entry.SpentOn,
	})
}

func (h *ToolHandlers) handleTrackersList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	trackers, err := h.resolver.GetTrackers()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list trackers: %v", err)), nil
	}

	result := make([]map[string]any, len(trackers))
	for i, t := range trackers {
		result[i] = map[string]any{
			"id":   t.ID,
			"name": t.Name,
		}
	}

	return jsonResult(map[string]any{
		"trackers": result,
		"count":    len(trackers),
	})
}

func (h *ToolHandlers) handleStatusesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	statuses, err := h.resolver.GetStatuses()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list statuses: %v", err)), nil
	}

	result := make([]map[string]any, len(statuses))
	for i, s := range statuses {
		result[i] = map[string]any{
			"id":        s.ID,
			"name":      s.Name,
			"is_closed": s.IsClosed,
		}
	}

	return jsonResult(map[string]any{
		"statuses": result,
		"count":    len(statuses),
	})
}

func (h *ToolHandlers) handleActivitiesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	activities, err := h.resolver.GetActivities()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list activities: %v", err)), nil
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

	return jsonResult(map[string]any{
		"activities": result,
		"count":      len(activities),
	})
}

// Helper functions

func (h *ToolHandlers) resolveCustomFields(fields map[string]any, projectID int) map[string]any {
	result := make(map[string]any)
	for name, value := range fields {
		// Try to resolve field name to ID by checking a sample issue
		// For now, just pass through - the API might accept field names
		if id, err := strconv.Atoi(name); err == nil {
			result[strconv.Itoa(id)] = value
		} else {
			// Keep the name, we'll need to resolve it
			result[name] = value
		}
	}
	return result
}

func formatIssue(issue redmine.Issue) map[string]any {
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

	if issue.StartDate != "" {
		result["start_date"] = issue.StartDate
	}
	if issue.DueDate != "" {
		result["due_date"] = issue.DueDate
	}
	if issue.ClosedOn != "" {
		result["closed_on"] = issue.ClosedOn
	}

	return result
}

func formatIssueDetail(issue redmine.Issue) map[string]any {
	result := formatIssue(issue)
	result["description"] = issue.Description
	result["done_ratio"] = issue.DoneRatio

	if issue.Parent != nil {
		result["parent_issue_id"] = issue.Parent.ID
	}

	// Custom fields
	if len(issue.CustomFields) > 0 {
		cf := make(map[string]any)
		for _, f := range issue.CustomFields {
			cf[f.Name] = f.Value
		}
		result["custom_fields"] = cf
	}

	// Journals (comments/changes)
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

	// Watchers
	if len(issue.Watchers) > 0 {
		watchers := make([]map[string]any, len(issue.Watchers))
		for i, w := range issue.Watchers {
			watchers[i] = map[string]any{
				"id":   w.ID,
				"name": w.Name,
			}
		}
		result["watchers"] = watchers
	}

	// Relations
	if len(issue.Relations) > 0 {
		relations := make([]map[string]any, len(issue.Relations))
		for i, r := range issue.Relations {
			relations[i] = map[string]any{
				"id":            r.ID,
				"issue_id":      r.IssueID,
				"issue_to_id":   r.IssueToID,
				"relation_type": r.RelationType,
			}
		}
		result["relations"] = relations
	}

	return result
}

func jsonResult(data any) (*mcp.CallToolResult, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func getMapArg(req mcp.CallToolRequest, key string) map[string]any {
	args := req.GetArguments()
	if v, ok := args[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}
