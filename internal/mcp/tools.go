package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

// resolveDatePeriod converts period shortcuts to from/to dates
func resolveDatePeriod(period string) (from, to string) {
	now := time.Now()

	switch period {
	case "this_week":
		// Start of this week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := now.AddDate(0, 0, -weekday+1)
		end := start.AddDate(0, 0, 6)
		return start.Format("2006-01-02"), end.Format("2006-01-02")
	case "last_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := now.AddDate(0, 0, -weekday-6)
		end := start.AddDate(0, 0, 6)
		return start.Format("2006-01-02"), end.Format("2006-01-02")
	case "this_month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return start.Format("2006-01-02"), end.Format("2006-01-02")
	case "last_month":
		start := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return start.Format("2006-01-02"), end.Format("2006-01-02")
	default:
		return "", ""
	}
}

// ToolHandlers contains all MCP tool handlers
type ToolHandlers struct {
	client   *redmine.Client
	resolver *redmine.Resolver
	rules    *redmine.CustomFieldRules
	workflow *redmine.WorkflowRules
}

// NewToolHandlers creates new tool handlers
func NewToolHandlers(client *redmine.Client, rules *redmine.CustomFieldRules, workflow *redmine.WorkflowRules) *ToolHandlers {
	return &ToolHandlers{
		client:   client,
		resolver: redmine.NewResolver(client),
		rules:    rules,
		workflow: workflow,
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
		mcp.WithNumber("parent_id",
			mcp.Description("Parent issue ID (find subtasks)"),
		),
		mcp.WithString("updated_after",
			mcp.Description("Only issues updated on or after this date (YYYY-MM-DD)"),
		),
		mcp.WithString("updated_before",
			mcp.Description("Only issues updated on or before this date (YYYY-MM-DD)"),
		),
		mcp.WithString("created_after",
			mcp.Description("Only issues created on or after this date (YYYY-MM-DD)"),
		),
		mcp.WithString("created_before",
			mcp.Description("Only issues created on or before this date (YYYY-MM-DD)"),
		),
		mcp.WithString("sort",
			mcp.Description("Sort order (e.g., 'updated_on:desc', 'priority:desc', 'created_on:asc')"),
		),
		mcp.WithObject("custom_fields",
			mcp.Description("Filter by custom field values (field name or ID -> value, e.g., {\"SW_Category\": \"SW Tool\"})"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of issues to return (default: 25)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset for pagination (default: 0)"),
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
		mcp.WithBoolean("is_private",
			mcp.Description("Whether the issue is private"),
		),
	), h.handleIssuesCreate)

	s.AddTool(mcp.NewTool("issues.update",
		mcp.WithDescription("Update an issue"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
		mcp.WithString("subject",
			mcp.Description("New subject/title"),
		),
		mcp.WithString("description",
			mcp.Description("New description"),
		),
		mcp.WithString("status",
			mcp.Description("New status name or ID"),
		),
		mcp.WithString("priority",
			mcp.Description("New priority name or ID (e.g., Low, Normal, High, Urgent, Immediate)"),
		),
		mcp.WithString("tracker",
			mcp.Description("New tracker name or ID"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("New assignee name or ID"),
		),
		mcp.WithString("start_date",
			mcp.Description("Start date (YYYY-MM-DD)"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date (YYYY-MM-DD)"),
		),
		mcp.WithString("notes",
			mcp.Description("Notes/comment to add"),
		),
		mcp.WithNumber("done_ratio",
			mcp.Description("Progress percentage (0-100)"),
		),
		mcp.WithObject("custom_fields",
			mcp.Description("Custom fields to update as key-value pairs"),
		),
		mcp.WithBoolean("is_private",
			mcp.Description("Whether the issue is private"),
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
		mcp.WithString("priority",
			mcp.Description("Priority name or ID (e.g., Low, Normal, High, Urgent, Immediate)"),
		),
		mcp.WithString("start_date",
			mcp.Description("Start date (YYYY-MM-DD)"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date (YYYY-MM-DD)"),
		),
		mcp.WithObject("custom_fields",
			mcp.Description("Custom fields as key-value pairs"),
		),
		mcp.WithBoolean("is_private",
			mcp.Description("Whether the subtask is private"),
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

	s.AddTool(mcp.NewTool("issues.getRequiredFields",
		mcp.WithDescription("Get required fields for creating an issue in a project/tracker"),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name or ID")),
		mcp.WithString("tracker", mcp.Required(), mcp.Description("Tracker name or ID")),
	), h.handleIssuesGetRequiredFields)

	// Custom Fields
	s.AddTool(mcp.NewTool("customFields.list",
		mcp.WithDescription("List custom fields available for a project/tracker"),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name or ID")),
		mcp.WithString("tracker", mcp.Description("Tracker name or ID (optional)")),
	), h.handleCustomFieldsList)

	s.AddTool(mcp.NewTool("customFields.listAll",
		mcp.WithDescription("List all custom field definitions (requires admin privileges). Use customFields.list for project-specific fields without admin access."),
		mcp.WithString("type",
			mcp.Description("Filter by customized type: issue, project, user, time_entry, version, group"),
		),
	), h.handleCustomFieldsListAll)

	// Projects (detail & update)
	s.AddTool(mcp.NewTool("projects.getDetail",
		mcp.WithDescription("Get project details including enabled trackers and custom fields"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
	), h.handleProjectsGetDetail)

	s.AddTool(mcp.NewTool("projects.update",
		mcp.WithDescription("Update project settings (trackers, custom fields, name, description). Requires admin or project manager privileges."),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("name",
			mcp.Description("New project name"),
		),
		mcp.WithString("description",
			mcp.Description("New project description"),
		),
		mcp.WithArray("tracker_ids",
			mcp.Description("Tracker names or IDs to enable for this project. Pass empty array to clear all."),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithArray("issue_custom_field_ids",
			mcp.Description("Custom field names or IDs to enable for this project. Pass empty array to clear all."),
			mcp.Items(map[string]any{"type": "string"}),
		),
	), h.handleProjectsUpdate)

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
		mcp.WithString("spent_on",
			mcp.Description("Date the time was spent (YYYY-MM-DD format, defaults to today)"),
		),
	), h.handleTimeEntriesCreate)

	s.AddTool(mcp.NewTool("timeEntries.list",
		mcp.WithDescription("List time entries with filters"),
		mcp.WithString("project", mcp.Description("Project name or ID")),
		mcp.WithString("user", mcp.Description("User name or ID, use 'me' for current user")),
		mcp.WithNumber("issue_id", mcp.Description("Filter by issue ID")),
		mcp.WithString("from", mcp.Description("Start date (YYYY-MM-DD)")),
		mcp.WithString("to", mcp.Description("End date (YYYY-MM-DD)")),
		mcp.WithString("period", mcp.Description("Date shortcut: this_week, last_week, this_month, last_month")),
		mcp.WithNumber("limit", mcp.Description("Results limit (default 25)")),
	), h.handleTimeEntriesList)

	s.AddTool(mcp.NewTool("timeEntries.report",
		mcp.WithDescription("Generate time entry report with aggregation"),
		mcp.WithString("project", mcp.Description("Project name or ID")),
		mcp.WithString("user", mcp.Description("User name or ID")),
		mcp.WithString("from", mcp.Description("Start date (YYYY-MM-DD)")),
		mcp.WithString("to", mcp.Description("End date (YYYY-MM-DD)")),
		mcp.WithString("period", mcp.Description("Date shortcut: this_week, last_week, this_month, last_month")),
		mcp.WithString("group_by", mcp.Required(), mcp.Description("Grouping: project, user, activity, or comma-separated combination")),
	), h.handleTimeEntriesReport)

	// Reference
	s.AddTool(mcp.NewTool("trackers.list",
		mcp.WithDescription("List all trackers"),
	), h.handleTrackersList)

	s.AddTool(mcp.NewTool("statuses.list",
		mcp.WithDescription("List all issue statuses"),
	), h.handleStatusesList)

	s.AddTool(mcp.NewTool("priorities.list",
		mcp.WithDescription("List all issue priorities"),
	), h.handlePrioritiesList)

	s.AddTool(mcp.NewTool("activities.list",
		mcp.WithDescription("List all time entry activities"),
	), h.handleActivitiesList)

	s.AddTool(mcp.NewTool("reference.workflow",
		mcp.WithDescription("Show workflow transition rules for trackers. Shows which status transitions are allowed for each tracker."),
		mcp.WithString("tracker",
			mcp.Description("Tracker name or ID (optional, shows all trackers if omitted)"),
		),
	), h.handleReferenceWorkflow)
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

	params.ParentID = req.GetInt("parent_id", 0)

	// Date filters: convert user-friendly params to Redmine filter syntax
	if after := req.GetString("updated_after", ""); after != "" {
		params.UpdatedOn = ">=" + after
	}
	if before := req.GetString("updated_before", ""); before != "" {
		if params.UpdatedOn != "" {
			// Both after and before: use range syntax "><start|end"
			after := req.GetString("updated_after", "")
			params.UpdatedOn = "><" + after + "|" + before
		} else {
			params.UpdatedOn = "<=" + before
		}
	}
	if after := req.GetString("created_after", ""); after != "" {
		params.CreatedOn = ">=" + after
	}
	if before := req.GetString("created_before", ""); before != "" {
		if params.CreatedOn != "" {
			after := req.GetString("created_after", "")
			params.CreatedOn = "><" + after + "|" + before
		} else {
			params.CreatedOn = "<=" + before
		}
	}

	params.Sort = req.GetString("sort", "")

	// Custom field filter: resolve field names to IDs
	if cfFilter := getMapArg(req, "custom_fields"); cfFilter != nil {
		params.CustomFieldFilter = make(map[string]string)
		for nameOrID, value := range cfFilter {
			cfID, err := h.resolver.ResolveCustomFieldByName(nameOrID, 0, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve custom field '%s': %v", nameOrID, err)), nil
			}
			params.CustomFieldFilter[strconv.Itoa(cfID)] = fmt.Sprintf("%v", value)
		}
	}

	params.Limit = req.GetInt("limit", 25)
	params.Offset = req.GetInt("offset", 0)

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

	// Add allowed_statuses from workflow rules (Redmine pre-5.0 doesn't provide this)
	if h.workflow != nil && len(issue.AllowedStatuses) == 0 {
		if allowed := h.workflow.GetAllowedStatuses(issue.Tracker.ID, issue.Status.ID); len(allowed) > 0 {
			statuses := make([]map[string]any, len(allowed))
			for i, s := range allowed {
				statuses[i] = map[string]any{"id": s.ID, "name": s.Name}
			}
			result["allowed_statuses"] = statuses
		}
	}

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

	if args := req.GetArguments(); args != nil {
		if v, ok := args["is_private"]; ok {
			if b, ok := v.(bool); ok {
				params.IsPrivate = &b
			}
		}
	}

	if customFields := getMapArg(req, "custom_fields"); customFields != nil {
		resolved, err := h.resolveCustomFields(customFields, projectID, trackerID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		params.CustomFields = resolved
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

	params.Subject = req.GetString("subject", "")
	params.Description = req.GetString("description", "")
	params.StartDate = req.GetString("start_date", "")
	params.DueDate = req.GetString("due_date", "")

	if priority := req.GetString("priority", ""); priority != "" {
		priorityID, err := h.resolver.ResolvePriority(priority)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve priority: %v", err)), nil
		}
		params.PriorityID = priorityID
	}

	if tracker := req.GetString("tracker", ""); tracker != "" {
		trackerID, err := h.resolver.ResolveTracker(tracker)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
		}
		params.TrackerID = trackerID
	}

	if status := req.GetString("status", ""); status != "" {
		statusID, err := h.resolver.ResolveStatusID(status)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve status: %v", err)), nil
		}
		if err := h.workflow.ValidateTransition(issue.Tracker.ID, issue.Status.ID, statusID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid status transition: %v", err)), nil
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

	// done_ratio and is_private: check if explicitly provided
	if args := req.GetArguments(); args != nil {
		if v, ok := args["done_ratio"]; ok {
			if f, ok := v.(float64); ok {
				ratio := int(f)
				params.DoneRatio = &ratio
			}
		}
		if v, ok := args["is_private"]; ok {
			if b, ok := v.(bool); ok {
				params.IsPrivate = &b
			}
		}
	}

	if customFields := getMapArg(req, "custom_fields"); customFields != nil {
		resolved, err := h.resolveCustomFields(customFields, issue.Project.ID, issue.Tracker.ID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		params.CustomFields = resolved
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
	params.StartDate = req.GetString("start_date", "")
	params.DueDate = req.GetString("due_date", "")

	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		userID, err := h.resolver.ResolveUser(assignedTo, parent.Project.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve assignee: %v", err)), nil
		}
		params.AssignedToID = userID
	}

	if priority := req.GetString("priority", ""); priority != "" {
		priorityID, err := h.resolver.ResolvePriority(priority)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve priority: %v", err)), nil
		}
		params.PriorityID = priorityID
	}

	if args := req.GetArguments(); args != nil {
		if v, ok := args["is_private"]; ok {
			if b, ok := v.(bool); ok {
				params.IsPrivate = &b
			}
		}
	}

	if customFields := getMapArg(req, "custom_fields"); customFields != nil {
		resolved, err := h.resolveCustomFields(customFields, parent.Project.ID, params.TrackerID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		params.CustomFields = resolved
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

func (h *ToolHandlers) handleIssuesGetRequiredFields(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	trackerStr, err := req.RequireString("tracker")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	trackerID, err := h.resolver.ResolveTracker(trackerStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
	}

	// Get project and tracker names for response
	projects, _ := h.client.ListProjects(100)
	trackers, _ := h.client.ListTrackers()

	var projectName, trackerName string
	for _, p := range projects {
		if p.ID == projectID {
			projectName = p.Name
			break
		}
	}
	for _, t := range trackers {
		if t.ID == trackerID {
			trackerName = t.Name
			break
		}
	}

	// Get custom fields
	customFields, err := h.client.GetProjectCustomFields(projectID, trackerID)
	if err != nil {
		// Return partial result even if custom fields unavailable
		return jsonResult(map[string]any{
			"project": projectName,
			"tracker": trackerName,
			"required": map[string]any{
				"standard": []string{"subject"},
				"custom":   []any{},
			},
			"note": "Could not retrieve custom fields: " + err.Error(),
		})
	}

	// Format custom fields
	customFieldsList := make([]map[string]any, 0)
	for _, cf := range customFields {
		field := map[string]any{
			"id":   cf.ID,
			"name": cf.Name,
		}
		if cf.FieldFormat != "" && cf.FieldFormat != "unknown" {
			field["type"] = cf.FieldFormat
		}
		if len(cf.PossibleValues) > 0 {
			field["possible_values"] = cf.PossibleValues
		}
		customFieldsList = append(customFieldsList, field)
	}

	return jsonResult(map[string]any{
		"project": projectName,
		"tracker": trackerName,
		"required": map[string]any{
			"standard": []string{"subject"},
			"custom":   customFieldsList,
		},
		"note": "Custom fields shown are available for this project/tracker. Required status cannot be determined without admin access - try creating the issue and check error messages.",
	})
}

func (h *ToolHandlers) handleCustomFieldsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	var trackerID int
	if tracker := req.GetString("tracker", ""); tracker != "" {
		trackerID, err = h.resolver.ResolveTracker(tracker)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
		}
	}

	fields, err := h.client.GetProjectCustomFields(projectID, trackerID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get custom fields: %v", err)), nil
	}

	// Format results
	results := make([]map[string]any, len(fields))
	for i, f := range fields {
		results[i] = map[string]any{
			"id":   f.ID,
			"name": f.Name,
		}
		if f.FieldFormat != "" && f.FieldFormat != "unknown" {
			results[i]["type"] = f.FieldFormat
		}
		if f.Required {
			results[i]["required"] = true
		}
		if len(f.PossibleValues) > 0 {
			results[i]["possible_values"] = f.PossibleValues
		}
	}

	return jsonResult(map[string]any{
		"project_id":    projectID,
		"tracker_id":    trackerID,
		"custom_fields": results,
	})
}

func (h *ToolHandlers) handleCustomFieldsListAll(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fields, err := h.client.ListAllCustomFields()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(
			"Requires admin privileges. Use customFields.list with a project/tracker to see fields available in a specific project (no admin required). Error: %v", err)), nil
	}

	// Optional type filter
	typeFilter := req.GetString("type", "")

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

	return jsonResult(map[string]any{
		"custom_fields": results,
		"count":         len(results),
	})
}

func (h *ToolHandlers) handleProjectsGetDetail(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	project, err := h.client.GetProjectDetail(projectID, []string{"trackers", "issue_custom_fields"})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get project detail: %v", err)), nil
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

	return jsonResult(result)
}

func (h *ToolHandlers) handleProjectsUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	params := redmine.UpdateProjectParams{
		ProjectID: projectID,
	}

	params.Name = req.GetString("name", "")
	params.Description = req.GetString("description", "")

	// Resolve tracker_ids (names or IDs)
	if trackerArgs := getArrayArg(req, "tracker_ids"); trackerArgs != nil {
		trackerIDs := make([]int, 0, len(trackerArgs))
		for _, arg := range trackerArgs {
			s := fmt.Sprintf("%v", arg)
			id, err := h.resolver.ResolveTracker(s)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker '%s': %v", s, err)), nil
			}
			trackerIDs = append(trackerIDs, id)
		}
		params.TrackerIDs = trackerIDs
	}

	// Resolve issue_custom_field_ids (names or IDs)
	if cfArgs := getArrayArg(req, "issue_custom_field_ids"); cfArgs != nil {
		cfIDs := make([]int, 0, len(cfArgs))
		for _, arg := range cfArgs {
			s := fmt.Sprintf("%v", arg)
			id, err := h.resolver.ResolveCustomFieldByName(s, projectID, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve custom field '%s': %v", s, err)), nil
			}
			cfIDs = append(cfIDs, id)
		}
		params.IssueCustomFieldIDs = cfIDs
	}

	if err := h.client.UpdateProject(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update project: %v", err)), nil
	}

	// Return updated project detail
	project, err := h.client.GetProjectDetail(projectID, []string{"trackers", "issue_custom_fields"})
	if err != nil {
		return jsonResult(map[string]any{
			"success":    true,
			"project_id": projectID,
			"message":    "Project updated successfully (could not fetch updated details)",
		})
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

	return jsonResult(result)
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
	params.SpentOn = req.GetString("spent_on", "")

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

func (h *ToolHandlers) handleTimeEntriesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := redmine.ListTimeEntriesParams{}

	// Handle project
	var projectID int
	if project := req.GetString("project", ""); project != "" {
		var err error
		projectID, err = h.resolver.ResolveProject(project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
		}
		params.ProjectID = strconv.Itoa(projectID)
	}

	// Handle user
	if user := req.GetString("user", ""); user != "" {
		if user == "me" {
			params.UserID = "me"
		} else {
			userID, err := h.resolver.ResolveUser(user, projectID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
			}
			params.UserID = strconv.Itoa(userID)
		}
	}

	// Handle issue_id
	if issueID := req.GetInt("issue_id", 0); issueID > 0 {
		params.IssueID = issueID
	}

	// Handle date period shortcut
	if period := req.GetString("period", ""); period != "" {
		params.From, params.To = resolveDatePeriod(period)
	}

	// Handle explicit from/to (override period if both specified)
	if from := req.GetString("from", ""); from != "" {
		params.From = from
	}
	if to := req.GetString("to", ""); to != "" {
		params.To = to
	}

	// Handle limit
	params.Limit = req.GetInt("limit", 25)

	entries, totalCount, err := h.client.ListTimeEntries(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list time entries: %v", err)), nil
	}

	// Format results
	results := make([]map[string]any, len(entries))
	for i, entry := range entries {
		result := map[string]any{
			"id":       entry.ID,
			"project":  entry.Project.Name,
			"user":     entry.User.Name,
			"activity": entry.Activity.Name,
			"hours":    entry.Hours,
			"spent_on": entry.SpentOn,
			"comments": entry.Comments,
		}
		if entry.Issue != nil {
			result["issue_id"] = entry.Issue.ID
		}
		results[i] = result
	}

	return jsonResult(map[string]any{
		"total_count":  totalCount,
		"count":        len(entries),
		"time_entries": results,
	})
}

func (h *ToolHandlers) handleTimeEntriesReport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Build params for fetching all matching entries
	params := redmine.ListTimeEntriesParams{
		Limit: 100, // Fetch in batches
	}

	// Handle project filter
	if project := req.GetString("project", ""); project != "" {
		projectID, err := h.resolver.ResolveProject(project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
		}
		params.ProjectID = strconv.Itoa(projectID)
	}

	// Handle user filter
	if user := req.GetString("user", ""); user != "" {
		if user == "me" {
			params.UserID = "me"
		} else {
			userID, err := h.resolver.ResolveUser(user, 0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
			}
			params.UserID = strconv.Itoa(userID)
		}
	}

	// Handle date period
	if period := req.GetString("period", ""); period != "" {
		params.From, params.To = resolveDatePeriod(period)
	}
	if from := req.GetString("from", ""); from != "" {
		params.From = from
	}
	if to := req.GetString("to", ""); to != "" {
		params.To = to
	}

	// Get grouping
	groupByStr, err := req.RequireString("group_by")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	groupBy := strings.Split(groupByStr, ",")
	for i := range groupBy {
		groupBy[i] = strings.TrimSpace(groupBy[i])
	}

	// Fetch all entries (paginated)
	var allEntries []redmine.TimeEntry
	for {
		entries, _, err := h.client.ListTimeEntries(params)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch time entries: %v", err)), nil
		}
		allEntries = append(allEntries, entries...)
		if len(entries) < params.Limit {
			break
		}
		params.Offset += params.Limit
	}

	// Aggregate by group_by
	type GroupKey struct {
		Project  string
		User     string
		Activity string
	}

	aggregated := make(map[GroupKey]float64)
	var totalHours float64

	for _, entry := range allEntries {
		key := GroupKey{}
		for _, g := range groupBy {
			switch g {
			case "project":
				key.Project = entry.Project.Name
			case "user":
				key.User = entry.User.Name
			case "activity":
				key.Activity = entry.Activity.Name
			}
		}
		aggregated[key] += entry.Hours
		totalHours += entry.Hours
	}

	// Build result
	groups := make([]map[string]any, 0, len(aggregated))
	for key, hours := range aggregated {
		group := map[string]any{
			"hours": hours,
		}
		if key.Project != "" {
			group["project"] = key.Project
		}
		if key.User != "" {
			group["user"] = key.User
		}
		if key.Activity != "" {
			group["activity"] = key.Activity
		}
		groups = append(groups, group)
	}

	// Sort by hours descending
	sort.Slice(groups, func(i, j int) bool {
		return groups[i]["hours"].(float64) > groups[j]["hours"].(float64)
	})

	result := map[string]any{
		"total_hours": totalHours,
		"entry_count": len(allEntries),
		"group_by":    groupBy,
		"groups":      groups,
	}

	if params.From != "" || params.To != "" {
		result["period"] = fmt.Sprintf("%s ~ %s", params.From, params.To)
	}

	return jsonResult(result)
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

func (h *ToolHandlers) handlePrioritiesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	priorities, err := h.resolver.GetPriorities()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list priorities: %v", err)), nil
	}

	result := make([]map[string]any, len(priorities))
	for i, p := range priorities {
		result[i] = map[string]any{
			"id":         p.ID,
			"name":       p.Name,
			"is_default": p.IsDefault,
		}
	}

	return jsonResult(map[string]any{
		"priorities": result,
		"count":      len(priorities),
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

func (h *ToolHandlers) handleReferenceWorkflow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.workflow == nil {
		return mcp.NewToolResultError("Workflow rules not configured. Set WORKFLOW_RULES_FILE or --workflow-rules flag."), nil
	}

	trackerStr := req.GetString("tracker", "")

	if trackerStr != "" {
		// Show specific tracker
		trackerID, err := h.resolver.ResolveTracker(trackerStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve tracker: %v", err)), nil
		}

		trackerKey := strconv.Itoa(trackerID)
		tracker, ok := h.workflow.Trackers[trackerKey]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("No workflow rules defined for tracker %s (ID: %d)", trackerStr, trackerID)), nil
		}

		return jsonResult(formatTrackerWorkflow(trackerID, tracker))
	}

	// Show all trackers summary
	trackers := make([]map[string]any, 0, len(h.workflow.Trackers))
	for idStr, tracker := range h.workflow.Trackers {
		id, _ := strconv.Atoi(idStr)
		trackers = append(trackers, formatTrackerWorkflow(id, tracker))
	}

	return jsonResult(map[string]any{
		"trackers": trackers,
		"count":    len(trackers),
	})
}

func formatTrackerWorkflow(trackerID int, tracker redmine.WorkflowTracker) map[string]any {
	// Build statuses list
	statuses := make([]map[string]any, 0, len(tracker.Statuses))
	for idStr, s := range tracker.Statuses {
		id, _ := strconv.Atoi(idStr)
		statuses = append(statuses, map[string]any{
			"id":        id,
			"name":      s.Name,
			"is_closed": s.IsClosed,
		})
	}

	// Build transitions
	transitions := make([]map[string]any, 0, len(tracker.Transitions))
	for fromIDStr, toIDs := range tracker.Transitions {
		fromID, _ := strconv.Atoi(fromIDStr)
		fromName := "Unknown"
		if s, ok := tracker.Statuses[fromIDStr]; ok {
			fromName = s.Name
		}

		targets := make([]map[string]any, len(toIDs))
		for i, toID := range toIDs {
			toName := "Unknown"
			if s, ok := tracker.Statuses[strconv.Itoa(toID)]; ok {
				toName = s.Name
			}
			targets[i] = map[string]any{"id": toID, "name": toName}
		}

		transitions = append(transitions, map[string]any{
			"from":    map[string]any{"id": fromID, "name": fromName},
			"allowed": targets,
		})
	}

	return map[string]any{
		"tracker_id":   trackerID,
		"tracker_name": tracker.Name,
		"statuses":     statuses,
		"transitions":  transitions,
	}
}

// Helper functions

// resolveCustomFields converts custom field names to IDs
func (h *ToolHandlers) resolveCustomFields(fields map[string]any, projectID int, trackerID int) (map[string]any, error) {
	result := make(map[string]any)
	var unknownFields []string

	// Try to get custom field definitions for name resolution
	definitions, defErr := h.client.GetProjectCustomFields(projectID, trackerID)
	nameToID := make(map[string]int)
	if defErr == nil {
		for _, def := range definitions {
			nameToID[strings.ToLower(def.Name)] = def.ID
		}
	}

	for key, value := range fields {
		var fieldID int

		// Try to parse as numeric ID first
		if id, err := strconv.Atoi(key); err == nil {
			fieldID = id
		} else if id, ok := nameToID[strings.ToLower(key)]; ok {
			// Resolve name to ID
			fieldID = id
		} else {
			// Unknown field
			unknownFields = append(unknownFields, key)
			continue
		}

		// Validate value against rules
		validated, err := h.validateCustomFieldValue(fieldID, value)
		if err != nil {
			return nil, err
		}
		result[strconv.Itoa(fieldID)] = validated
	}

	if len(unknownFields) > 0 {
		// Build helpful error message
		availableFields := make([]string, 0, len(nameToID))
		for name := range nameToID {
			availableFields = append(availableFields, name)
		}
		sort.Strings(availableFields)

		return nil, fmt.Errorf("custom field(s) not found: %s\nAvailable fields: %s",
			strings.Join(unknownFields, ", "),
			strings.Join(availableFields, ", "))
	}

	return result, nil
}

// validateCustomFieldValue validates and auto-corrects a custom field value.
// Handles both single string values and array values (multi-select fields).
func (h *ToolHandlers) validateCustomFieldValue(fieldID int, value any) (any, error) {
	if h.rules == nil {
		return value, nil
	}

	switch v := value.(type) {
	case string:
		return h.rules.ValidateValue(fieldID, v)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				corrected, err := h.rules.ValidateValue(fieldID, s)
				if err != nil {
					return nil, err
				}
				result[i] = corrected
			} else {
				result[i] = item
			}
		}
		return result, nil
	default:
		return value, nil
	}
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

	// Allowed status transitions
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

func getArrayArg(req mcp.CallToolRequest, key string) []any {
	args := req.GetArguments()
	if v, ok := args[key]; ok {
		if arr, ok := v.([]any); ok {
			return arr
		}
	}
	return nil
}
