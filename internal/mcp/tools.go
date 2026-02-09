package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
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
	readOnly bool
}

// NewToolHandlers creates new tool handlers
func NewToolHandlers(client *redmine.Client, rules *redmine.CustomFieldRules, workflow *redmine.WorkflowRules) *ToolHandlers {
	readOnly := os.Getenv("REDMINE_MCP_READ_ONLY") == "true"
	if readOnly {
		slog.Info("read-only mode enabled - all write operations will be blocked")
	}
	return &ToolHandlers{
		client:   client,
		resolver: redmine.NewResolver(client),
		rules:    rules,
		workflow: workflow,
		readOnly: readOnly,
	}
}

// referenceData holds enum values fetched from Redmine for tool parameter hints.
type referenceData struct {
	trackers   []string
	statuses   []string
	priorities []string
	activities []string
}

// fetchReferenceData loads tracker/status/priority/activity names from Redmine.
// Never fails — returns empty slices on error so tools still register without enums.
func (h *ToolHandlers) fetchReferenceData() *referenceData {
	ref := &referenceData{}

	if trackers, err := h.resolver.GetTrackers(); err != nil {
		slog.Warn("failed to fetch trackers for enum hints", "error", err)
	} else {
		for _, t := range trackers {
			ref.trackers = append(ref.trackers, t.Name)
		}
	}

	if statuses, err := h.resolver.GetStatuses(); err != nil {
		slog.Warn("failed to fetch statuses for enum hints", "error", err)
	} else {
		for _, s := range statuses {
			ref.statuses = append(ref.statuses, s.Name)
		}
	}

	if priorities, err := h.resolver.GetPriorities(); err != nil {
		slog.Warn("failed to fetch priorities for enum hints", "error", err)
	} else {
		for _, p := range priorities {
			ref.priorities = append(ref.priorities, p.Name)
		}
	}

	if activities, err := h.resolver.GetActivities(); err != nil {
		slog.Warn("failed to fetch activities for enum hints", "error", err)
	} else {
		for _, a := range activities {
			ref.activities = append(ref.activities, a.Name)
		}
	}

	return ref
}

// enumOpt returns a slice containing an mcp.Enum option if values is non-empty,
// or nil otherwise. Designed for use with append() into variadic option lists.
func enumOpt(values []string) []mcp.PropertyOption {
	if len(values) == 0 {
		return nil
	}
	return []mcp.PropertyOption{mcp.Enum(values...)}
}

// checkReadOnly returns an error if the server is in read-only mode.
func (h *ToolHandlers) checkReadOnly() error {
	if h.readOnly {
		return fmt.Errorf("server is in read-only mode - write operations are disabled")
	}
	return nil
}

// RegisterTools registers all MCP tools on the server
func (h *ToolHandlers) RegisterTools(s McpServer) {
	ref := h.fetchReferenceData()
	// Account
	s.AddTool(mcp.NewTool("me",
		mcp.WithDescription("Get current user information"),
	), h.handleMe)

	// Projects
	s.AddTool(mcp.NewTool("projects_list",
		mcp.WithDescription("List all projects"),
		mcp.WithNumber("limit",
			mcp.Description("Number of projects to return (default: 100)"),
		),
	), h.handleProjectsList)

	s.AddTool(mcp.NewTool("projects_create",
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
	searchStatuses := append([]string{"open", "closed", "*"}, ref.statuses...)

	s.AddTool(mcp.NewTool("issues_search",
		mcp.WithDescription("Search issues"),
		mcp.WithString("project",
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Description("Tracker name or ID"),
			}, enumOpt(ref.trackers)...)...,
		),
		mcp.WithString("status",
			append([]mcp.PropertyOption{
				mcp.Description("Status: open, closed, all, or specific status name"),
			}, enumOpt(searchStatuses)...)...,
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

	s.AddTool(mcp.NewTool("issues_getById",
		mcp.WithDescription("Get issue details including journals, watchers, and relations"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
	), h.handleIssuesGetById)

	s.AddTool(mcp.NewTool("issues_create",
		mcp.WithDescription("Create a new issue"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Required(),
				mcp.Description("Tracker name or ID"),
			}, enumOpt(ref.trackers)...)...,
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
		mcp.WithArray("upload_tokens",
			mcp.Description("Upload tokens from attachments_upload to attach files (array of {token, filename, content_type, description})"),
			mcp.Items(map[string]any{"type": "object"}),
		),
	), h.handleIssuesCreate)

	s.AddTool(mcp.NewTool("issues_update",
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
			append([]mcp.PropertyOption{
				mcp.Description("New status name or ID"),
			}, enumOpt(ref.statuses)...)...,
		),
		mcp.WithString("priority",
			append([]mcp.PropertyOption{
				mcp.Description("New priority name or ID"),
			}, enumOpt(ref.priorities)...)...,
		),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Description("New tracker name or ID"),
			}, enumOpt(ref.trackers)...)...,
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
		mcp.WithArray("upload_tokens",
			mcp.Description("Upload tokens from attachments_upload to attach files (array of {token, filename, content_type, description})"),
			mcp.Items(map[string]any{"type": "object"}),
		),
	), h.handleIssuesUpdate)

	s.AddTool(mcp.NewTool("issues_createSubtask",
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
			append([]mcp.PropertyOption{
				mcp.Description("Tracker name or ID (defaults to parent's tracker)"),
			}, enumOpt(ref.trackers)...)...,
		),
		mcp.WithString("description",
			mcp.Description("Subtask description"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("Assignee name or ID"),
		),
		mcp.WithString("priority",
			append([]mcp.PropertyOption{
				mcp.Description("Priority name or ID"),
			}, enumOpt(ref.priorities)...)...,
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

	s.AddTool(mcp.NewTool("issues_addWatcher",
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

	s.AddTool(mcp.NewTool("issues_addRelation",
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

	s.AddTool(mcp.NewTool("issues_getRequiredFields",
		mcp.WithDescription("Get required fields for creating an issue in a project/tracker"),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name or ID")),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Required(),
				mcp.Description("Tracker name or ID"),
			}, enumOpt(ref.trackers)...)...,
		),
	), h.handleIssuesGetRequiredFields)

	// Custom Fields
	s.AddTool(mcp.NewTool("customFields_list",
		mcp.WithDescription("List custom fields available for a project/tracker"),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name or ID")),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Description("Tracker name or ID (optional)"),
			}, enumOpt(ref.trackers)...)...,
		),
	), h.handleCustomFieldsList)

	s.AddTool(mcp.NewTool("customFields_listAll",
		mcp.WithDescription("List all custom field definitions (requires admin privileges). Use customFields_list for project-specific fields without admin access."),
		mcp.WithString("type",
			mcp.Description("Filter by customized type: issue, project, user, time_entry, version, group"),
		),
	), h.handleCustomFieldsListAll)

	// Projects (detail & update)
	s.AddTool(mcp.NewTool("projects_getDetail",
		mcp.WithDescription("Get project details including enabled trackers and custom fields"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
	), h.handleProjectsGetDetail)

	s.AddTool(mcp.NewTool("projects_update",
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
	s.AddTool(mcp.NewTool("timeEntries_create",
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
			append([]mcp.PropertyOption{
				mcp.Description("Activity name or ID"),
			}, enumOpt(ref.activities)...)...,
		),
		mcp.WithString("comments",
			mcp.Description("Comments"),
		),
		mcp.WithString("spent_on",
			mcp.Description("Date the time was spent (YYYY-MM-DD format, defaults to today)"),
		),
	), h.handleTimeEntriesCreate)

	s.AddTool(mcp.NewTool("timeEntries_list",
		mcp.WithDescription("List time entries with filters"),
		mcp.WithString("project", mcp.Description("Project name or ID")),
		mcp.WithString("user", mcp.Description("User name or ID, use 'me' for current user")),
		mcp.WithNumber("issue_id", mcp.Description("Filter by issue ID")),
		mcp.WithString("from", mcp.Description("Start date (YYYY-MM-DD)")),
		mcp.WithString("to", mcp.Description("End date (YYYY-MM-DD)")),
		mcp.WithString("period", mcp.Description("Date shortcut: this_week, last_week, this_month, last_month")),
		mcp.WithNumber("limit", mcp.Description("Results limit (default 25)")),
	), h.handleTimeEntriesList)

	s.AddTool(mcp.NewTool("timeEntries_report",
		mcp.WithDescription("Generate time entry report with aggregation"),
		mcp.WithString("project", mcp.Description("Project name or ID")),
		mcp.WithString("user", mcp.Description("User name or ID")),
		mcp.WithString("from", mcp.Description("Start date (YYYY-MM-DD)")),
		mcp.WithString("to", mcp.Description("End date (YYYY-MM-DD)")),
		mcp.WithString("period", mcp.Description("Date shortcut: this_week, last_week, this_month, last_month")),
		mcp.WithString("group_by", mcp.Required(), mcp.Description("Grouping: project, user, activity, or comma-separated combination")),
	), h.handleTimeEntriesReport)

	// Attachments
	s.AddTool(mcp.NewTool("attachments_upload",
		mcp.WithDescription("Upload a file to Redmine and get an upload token. Use the token with issues_create or issues_update to attach the file."),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("Filename (e.g., 'report.pdf')"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("File content as base64-encoded string (max 3MB decoded)"),
		),
		mcp.WithString("content_type",
			mcp.Description("MIME type (e.g., 'application/pdf'). Auto-detected if omitted."),
		),
	), h.handleAttachmentsUpload)

	s.AddTool(mcp.NewTool("attachments_download",
		mcp.WithDescription("Download an attachment by ID. Returns base64-encoded content."),
		mcp.WithNumber("attachment_id",
			mcp.Required(),
			mcp.Description("Attachment ID"),
		),
	), h.handleAttachmentsDownload)

	s.AddTool(mcp.NewTool("attachments_list",
		mcp.WithDescription("List attachments on an issue"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
	), h.handleAttachmentsList)

	s.AddTool(mcp.NewTool("attachments_uploadAndAttach",
		mcp.WithDescription("Upload a file and attach it to an issue in one step. Most common use case for adding attachments."),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID to attach the file to"),
		),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("Filename (e.g., 'report.pdf')"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("File content as base64-encoded string (max 3MB decoded)"),
		),
		mcp.WithString("content_type",
			mcp.Description("MIME type (e.g., 'application/pdf'). Auto-detected if omitted."),
		),
		mcp.WithString("description",
			mcp.Description("File description"),
		),
		mcp.WithString("notes",
			mcp.Description("Notes/comment to add to the issue along with the attachment"),
		),
	), h.handleAttachmentsUploadAndAttach)

	// Reference
	s.AddTool(mcp.NewTool("trackers_list",
		mcp.WithDescription("List all trackers"),
	), h.handleTrackersList)

	s.AddTool(mcp.NewTool("statuses_list",
		mcp.WithDescription("List all issue statuses"),
	), h.handleStatusesList)

	s.AddTool(mcp.NewTool("priorities_list",
		mcp.WithDescription("List all issue priorities"),
	), h.handlePrioritiesList)

	s.AddTool(mcp.NewTool("activities_list",
		mcp.WithDescription("List all time entry activities"),
	), h.handleActivitiesList)

	s.AddTool(mcp.NewTool("roles_list",
		mcp.WithDescription("List all roles available in the Redmine instance"),
	), h.handleRolesList)

	s.AddTool(mcp.NewTool("reference_workflow",
		mcp.WithDescription("Show workflow transition rules for trackers. Shows which status transitions are allowed for each tracker."),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Description("Tracker name or ID (optional, shows all trackers if omitted)"),
			}, enumOpt(ref.trackers)...)...,
		),
	), h.handleReferenceWorkflow)

	// --- Group A: CRUD Gaps ---

	s.AddTool(mcp.NewTool("timeEntries_update",
		mcp.WithDescription("Update an existing time entry"),
		mcp.WithNumber("time_entry_id",
			mcp.Required(),
			mcp.Description("Time entry ID"),
		),
		mcp.WithNumber("hours",
			mcp.Description("Hours spent"),
		),
		mcp.WithString("activity",
			append([]mcp.PropertyOption{
				mcp.Description("Activity name or ID"),
			}, enumOpt(ref.activities)...)...,
		),
		mcp.WithString("comments",
			mcp.Description("Comments"),
		),
		mcp.WithString("spent_on",
			mcp.Description("Date the time was spent (YYYY-MM-DD)"),
		),
	), h.handleTimeEntriesUpdate)

	s.AddTool(mcp.NewTool("timeEntries_delete",
		mcp.WithDescription("Delete a time entry"),
		mcp.WithNumber("time_entry_id",
			mcp.Required(),
			mcp.Description("Time entry ID"),
		),
	), h.handleTimeEntriesDelete)

	s.AddTool(mcp.NewTool("issues_removeWatcher",
		mcp.WithDescription("Remove a watcher from an issue"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Issue ID"),
		),
		mcp.WithString("user",
			mcp.Required(),
			mcp.Description("User name or ID to remove as watcher"),
		),
	), h.handleIssuesRemoveWatcher)

	s.AddTool(mcp.NewTool("issues_removeRelation",
		mcp.WithDescription("Remove a relation between issues"),
		mcp.WithNumber("relation_id",
			mcp.Required(),
			mcp.Description("Relation ID"),
		),
	), h.handleIssuesRemoveRelation)

	// --- Group E: User Search ---

	s.AddTool(mcp.NewTool("users_search",
		mcp.WithDescription("Search for users by name, project membership, or status"),
		mcp.WithString("name",
			mcp.Description("Search by user name (partial match)"),
		),
		mcp.WithString("project",
			mcp.Description("Project name or ID to search within"),
		),
		mcp.WithNumber("status",
			mcp.Description("1=active, 2=registered, 3=locked"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of results to return (default: 25)"),
		),
	), h.handleUsersSearch)

	// --- Global Search ---

	s.AddTool(mcp.NewTool("search_global",
		mcp.WithDescription("Search across all Redmine resources (issues, wiki, news, documents, changesets, messages, projects)"),
		mcp.WithString("q",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithString("scope",
			mcp.Description("Search scope: all (default), my_projects, subprojects"),
		),
		mcp.WithBoolean("titles_only",
			mcp.Description("Match only in titles (default: false)"),
		),
		mcp.WithBoolean("issues",
			mcp.Description("Include issues in results"),
		),
		mcp.WithBoolean("wiki_pages",
			mcp.Description("Include wiki pages in results"),
		),
		mcp.WithBoolean("news",
			mcp.Description("Include news in results"),
		),
		mcp.WithBoolean("documents",
			mcp.Description("Include documents in results"),
		),
		mcp.WithBoolean("changesets",
			mcp.Description("Include changesets in results"),
		),
		mcp.WithBoolean("messages",
			mcp.Description("Include forum messages in results"),
		),
		mcp.WithBoolean("projects",
			mcp.Description("Include projects in results"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset for pagination (default: 0)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max results to return (default: 25)"),
		),
	), h.handleSearchGlobal)

	// --- Group B: Batch & Copy ---

	s.AddTool(mcp.NewTool("issues_batchUpdate",
		mcp.WithDescription("Update multiple issues at once. Continues on individual failures (partial success)."),
		mcp.WithArray("issue_ids",
			mcp.Required(),
			mcp.Description("Array of issue IDs to update"),
			mcp.Items(map[string]any{"type": "number"}),
		),
		mcp.WithString("status",
			append([]mcp.PropertyOption{
				mcp.Description("New status name or ID"),
			}, enumOpt(ref.statuses)...)...,
		),
		mcp.WithString("assigned_to",
			mcp.Description("New assignee name or ID"),
		),
		mcp.WithString("priority",
			append([]mcp.PropertyOption{
				mcp.Description("New priority name or ID"),
			}, enumOpt(ref.priorities)...)...,
		),
		mcp.WithString("notes",
			mcp.Description("Notes/comment to add to each issue"),
		),
	), h.handleIssuesBatchUpdate)

	s.AddTool(mcp.NewTool("issues_copy",
		mcp.WithDescription("Copy an issue, optionally to a different project or with a new subject"),
		mcp.WithNumber("issue_id",
			mcp.Required(),
			mcp.Description("Source issue ID to copy"),
		),
		mcp.WithString("project",
			mcp.Description("Target project name or ID (defaults to same project)"),
		),
		mcp.WithString("subject",
			mcp.Description("Override subject for the copy (defaults to source subject)"),
		),
	), h.handleIssuesCopy)

	// --- Group C: Versions ---

	s.AddTool(mcp.NewTool("versions_list",
		mcp.WithDescription("List all versions/milestones for a project"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
	), h.handleVersionsList)

	s.AddTool(mcp.NewTool("versions_create",
		mcp.WithDescription("Create a new version/milestone in a project"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Version name"),
		),
		mcp.WithString("description",
			mcp.Description("Version description"),
		),
		mcp.WithString("status",
			mcp.Description("Version status"),
			mcp.Enum("open", "locked", "closed"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date (YYYY-MM-DD)"),
		),
		mcp.WithString("sharing",
			mcp.Description("Sharing scope"),
			mcp.Enum("none", "descendants", "hierarchy", "tree", "system"),
		),
	), h.handleVersionsCreate)

	s.AddTool(mcp.NewTool("versions_update",
		mcp.WithDescription("Update an existing version/milestone"),
		mcp.WithNumber("version_id",
			mcp.Required(),
			mcp.Description("Version ID"),
		),
		mcp.WithString("name",
			mcp.Description("Version name"),
		),
		mcp.WithString("description",
			mcp.Description("Version description"),
		),
		mcp.WithString("status",
			mcp.Description("Version status"),
			mcp.Enum("open", "locked", "closed"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date (YYYY-MM-DD)"),
		),
		mcp.WithString("sharing",
			mcp.Description("Sharing scope"),
			mcp.Enum("none", "descendants", "hierarchy", "tree", "system"),
		),
	), h.handleVersionsUpdate)

	// --- Issue Categories ---

	s.AddTool(mcp.NewTool("categories_list",
		mcp.WithDescription("List all issue categories for a project"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
	), h.handleCategoriesList)

	s.AddTool(mcp.NewTool("categories_create",
		mcp.WithDescription("Create a new issue category in a project"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Category name"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("Default assignee name or ID for issues in this category"),
		),
	), h.handleCategoriesCreate)

	s.AddTool(mcp.NewTool("categories_update",
		mcp.WithDescription("Update an existing issue category"),
		mcp.WithNumber("category_id",
			mcp.Required(),
			mcp.Description("Category ID"),
		),
		mcp.WithString("name",
			mcp.Description("New category name"),
		),
		mcp.WithString("assigned_to",
			mcp.Description("Default assignee name or ID"),
		),
	), h.handleCategoriesUpdate)

	s.AddTool(mcp.NewTool("categories_delete",
		mcp.WithDescription("Delete an issue category"),
		mcp.WithNumber("category_id",
			mcp.Required(),
			mcp.Description("Category ID"),
		),
	), h.handleCategoriesDelete)

	// --- Project Memberships ---

	s.AddTool(mcp.NewTool("memberships_list",
		mcp.WithDescription("List all memberships (users and groups) for a project"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
	), h.handleMembershipsList)

	s.AddTool(mcp.NewTool("memberships_add",
		mcp.WithDescription("Add a user or group to a project with specified roles"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("user",
			mcp.Description("User name or ID (specify either user or group, not both)"),
		),
		mcp.WithString("group",
			mcp.Description("Group name or ID (specify either user or group, not both)"),
		),
		mcp.WithArray("roles",
			mcp.Required(),
			mcp.Description("Array of role names or IDs to assign"),
		),
	), h.handleMembershipsAdd)

	s.AddTool(mcp.NewTool("memberships_update",
		mcp.WithDescription("Update roles for an existing membership"),
		mcp.WithNumber("membership_id",
			mcp.Required(),
			mcp.Description("Membership ID"),
		),
		mcp.WithArray("roles",
			mcp.Required(),
			mcp.Description("Array of role names or IDs to assign"),
		),
	), h.handleMembershipsUpdate)

	s.AddTool(mcp.NewTool("memberships_remove",
		mcp.WithDescription("Remove a membership from a project"),
		mcp.WithNumber("membership_id",
			mcp.Required(),
			mcp.Description("Membership ID"),
		),
	), h.handleMembershipsRemove)

	// --- Group D: Wiki ---

	s.AddTool(mcp.NewTool("wiki_list",
		mcp.WithDescription("List all wiki pages for a project"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
	), h.handleWikiList)

	s.AddTool(mcp.NewTool("wiki_get",
		mcp.WithDescription("Get a wiki page with its content"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Wiki page title"),
		),
	), h.handleWikiGet)

	s.AddTool(mcp.NewTool("wiki_createOrUpdate",
		mcp.WithDescription("Create or update a wiki page"),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Wiki page title"),
		),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("Wiki page content (Textile or Markdown depending on Redmine config)"),
		),
		mcp.WithString("comments",
			mcp.Description("Edit comment / version note"),
		),
	), h.handleWikiCreateOrUpdate)

	// --- Group F: Export ---

	s.AddTool(mcp.NewTool("issues_exportCSV",
		mcp.WithDescription("Export issues as CSV text. Same filters as issues_search."),
		mcp.WithString("project",
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("tracker",
			append([]mcp.PropertyOption{
				mcp.Description("Tracker name or ID"),
			}, enumOpt(ref.trackers)...)...,
		),
		mcp.WithString("status",
			append([]mcp.PropertyOption{
				mcp.Description("Status: open, closed, all, or specific status name"),
			}, enumOpt(searchStatuses)...)...,
		),
		mcp.WithString("assigned_to",
			mcp.Description("Assignee name or 'me' for current user"),
		),
		mcp.WithString("subject",
			mcp.Description("Search keyword in issue subject (partial match)"),
		),
		mcp.WithString("sort",
			mcp.Description("Sort order (e.g., 'updated_on:desc')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Number of issues to return (default: 25)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset for pagination (default: 0)"),
		),
	), h.handleIssuesExportCSV)

	// --- Group G: Reports ---

	s.AddTool(mcp.NewTool("reports_weekly",
		mcp.WithDescription("Generate a weekly time report aggregated by day, issue, and activity"),
		mcp.WithString("user",
			mcp.Description("User name or ID (default: 'me')"),
		),
		mcp.WithString("week_of",
			mcp.Description("Any date within the target week (YYYY-MM-DD, defaults to current week)"),
		),
	), h.handleReportsWeekly)

	s.AddTool(mcp.NewTool("reports_standup",
		mcp.WithDescription("Generate a standup report: yesterday's time entries + today's open issues"),
		mcp.WithString("user",
			mcp.Description("User name or ID (default: 'me')"),
		),
		mcp.WithString("date",
			mcp.Description("Date for the standup (YYYY-MM-DD, defaults to today)"),
		),
	), h.handleReportsStandup)

	s.AddTool(mcp.NewTool("reports_project_analysis",
		mcp.WithDescription("Generate comprehensive project analysis report with time tracking, issue statistics, and custom field breakdowns. Useful for project retrospectives and resource planning."),
		mcp.WithString("project",
			mcp.Required(),
			mcp.Description("Project name or ID"),
		),
		mcp.WithString("from",
			mcp.Description("Start date (YYYY-MM-DD)"),
		),
		mcp.WithString("to",
			mcp.Description("End date (YYYY-MM-DD)"),
		),
		mcp.WithString("issue_status",
			mcp.Description("Filter by issue status: 'all' (default), 'open', 'closed'"),
		),
		mcp.WithString("version",
			mcp.Description("Filter by version/milestone name or ID"),
		),
		mcp.WithString("custom_fields",
			mcp.Description("Custom fields to analyze (comma-separated). Default: all custom fields"),
		),
		mcp.WithString("format",
			mcp.Description("Output format: 'json' (default), 'csv'"),
		),
		mcp.WithString("attach_to",
			mcp.Description("Where to save the report: 'dmsf' (DMSF plugin, recommended), 'dmsf:FolderID', 'files', 'files:VersionName', 'issue:ID'. If omitted, returns data directly"),
		),
	), h.handleReportsProjectAnalysis)
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
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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

	customFields := getMapArg(req, "custom_fields")
	if customFields == nil {
		customFields = map[string]any{}
	}
	resolved, err := h.resolveCustomFields(customFields, projectID, trackerID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	params.CustomFields = resolved

	if tokens := getArrayArg(req, "upload_tokens"); tokens != nil {
		uploads, err := parseUploadTokens(tokens)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		params.Uploads = uploads
	}

	issue, err := h.client.CreateIssue(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create issue: %v", err)), nil
	}

	return jsonResult(formatIssue(*issue))
}

func (h *ToolHandlers) handleIssuesUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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

	if tokens := getArrayArg(req, "upload_tokens"); tokens != nil {
		uploads, err := parseUploadTokens(tokens)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		params.Uploads = uploads
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
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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

	customFields := getMapArg(req, "custom_fields")
	if customFields == nil {
		customFields = map[string]any{}
	}
	resolved, err := h.resolveCustomFields(customFields, parent.Project.ID, params.TrackerID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	params.CustomFields = resolved

	issue, err := h.client.CreateIssue(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create subtask: %v", err)), nil
	}

	return jsonResult(formatIssue(*issue))
}

func (h *ToolHandlers) handleIssuesAddWatcher(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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
			"Requires admin privileges. Use customFields_list with a project/tracker to see fields available in a specific project (no admin required). Error: %v", err)), nil
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
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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

const maxMCPAttachmentSize = 3 * 1024 * 1024 // 3MB decoded

func (h *ToolHandlers) handleAttachmentsUpload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	filename, err := req.RequireString("filename")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	contentB64, err := req.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid base64 content: %v", err)), nil
	}

	if len(decoded) > maxMCPAttachmentSize {
		return mcp.NewToolResultError(fmt.Sprintf("File too large: %d bytes (max %d bytes / 3MB)", len(decoded), maxMCPAttachmentSize)), nil
	}

	token, err := h.client.UploadFile(filename, bytes.NewReader(decoded))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to upload file: %v", err)), nil
	}

	contentType := req.GetString("content_type", "")
	if contentType != "" {
		token.ContentType = contentType
	}

	return jsonResult(map[string]any{
		"token":    token.Token,
		"filename": token.Filename,
		"size":     len(decoded),
		"message":  "File uploaded. Use the token with issues_create or issues_update to attach it.",
	})
}

func (h *ToolHandlers) handleAttachmentsDownload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idFloat, err := req.RequireFloat("attachment_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	attachmentID := int(idFloat)

	data, contentType, filename, err := h.client.DownloadAttachment(attachmentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to download attachment: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"attachment_id": attachmentID,
		"filename":      filename,
		"content_type":  contentType,
		"size":          len(data),
		"content":       base64.StdEncoding.EncodeToString(data),
	})
}

func (h *ToolHandlers) handleAttachmentsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(idFloat)

	issue, err := h.client.GetIssue(issueID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get issue: %v", err)), nil
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

	return jsonResult(map[string]any{
		"issue_id":    issueID,
		"attachments": attachments,
		"count":       len(attachments),
	})
}

func (h *ToolHandlers) handleAttachmentsUploadAndAttach(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	issueID := int(issueIDFloat)

	filename, err := req.RequireString("filename")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	contentB64, err := req.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid base64 content: %v", err)), nil
	}

	if len(decoded) > maxMCPAttachmentSize {
		return mcp.NewToolResultError(fmt.Sprintf("File too large: %d bytes (max %d bytes / 3MB)", len(decoded), maxMCPAttachmentSize)), nil
	}

	// Step 1: Upload
	token, err := h.client.UploadFile(filename, bytes.NewReader(decoded))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to upload file: %v", err)), nil
	}

	contentType := req.GetString("content_type", "")
	if contentType != "" {
		token.ContentType = contentType
	}
	token.Description = req.GetString("description", "")

	// Step 2: Attach to issue
	params := redmine.UpdateIssueParams{
		IssueID: issueID,
		Notes:   req.GetString("notes", ""),
		Uploads: []redmine.UploadToken{*token},
	}

	if err := h.client.UpdateIssue(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("File uploaded (token: %s) but failed to attach to issue: %v", token.Token, err)), nil
	}

	return jsonResult(map[string]any{
		"success":  true,
		"issue_id": issueID,
		"filename": filename,
		"size":     len(decoded),
		"message":  "File uploaded and attached to issue successfully",
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

func (h *ToolHandlers) handleRolesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	roles, err := h.resolver.GetRoles()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list roles: %v", err)), nil
	}

	result := make([]map[string]any, len(roles))
	for i, r := range roles {
		result[i] = map[string]any{
			"id":   r.ID,
			"name": r.Name,
		}
	}

	return jsonResult(map[string]any{
		"roles": result,
		"count": len(roles),
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

// --- Group A: CRUD Gaps ---

func (h *ToolHandlers) handleTimeEntriesUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("time_entry_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeEntryID := int(idFloat)

	params := redmine.UpdateTimeEntryParams{
		TimeEntryID: timeEntryID,
	}

	if args := req.GetArguments(); args != nil {
		if v, ok := args["hours"]; ok {
			if f, ok := v.(float64); ok {
				params.Hours = f
			}
		}
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

	if err := h.client.UpdateTimeEntry(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update time entry: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":       true,
		"time_entry_id": timeEntryID,
		"message":       "Time entry updated successfully",
	})
}

func (h *ToolHandlers) handleTimeEntriesDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("time_entry_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeEntryID := int(idFloat)

	if err := h.client.DeleteTimeEntry(timeEntryID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete time entry: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":       true,
		"time_entry_id": timeEntryID,
		"message":       "Time entry deleted successfully",
	})
}

func (h *ToolHandlers) handleIssuesRemoveWatcher(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

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

	if err := h.client.RemoveWatcher(issueID, userID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to remove watcher: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":  true,
		"issue_id": issueID,
		"user_id":  userID,
		"message":  "Watcher removed successfully",
	})
}

func (h *ToolHandlers) handleIssuesRemoveRelation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("relation_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	relationID := int(idFloat)

	if err := h.client.DeleteRelation(relationID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to remove relation: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":     true,
		"relation_id": relationID,
		"message":     "Relation removed successfully",
	})
}

// --- Group E: User Search ---

func (h *ToolHandlers) handleUsersSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := redmine.SearchUsersParams{}

	params.Name = req.GetString("name", "")

	if project := req.GetString("project", ""); project != "" {
		projectID, err := h.resolver.ResolveProject(project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
		}
		params.ProjectID = projectID
	}

	params.Status = req.GetInt("status", 0)
	params.Limit = req.GetInt("limit", 25)

	users, total, err := h.client.SearchUsers(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search users: %v", err)), nil
	}

	results := make([]map[string]any, len(users))
	for i, u := range users {
		result := map[string]any{
			"id":   u.ID,
			"name": u.Name,
		}
		if u.Login != "" {
			result["login"] = u.Login
		}
		if u.Mail != "" {
			result["email"] = u.Mail
		}
		results[i] = result
	}

	return jsonResult(map[string]any{
		"users":       results,
		"count":       len(users),
		"total_count": total,
	})
}

// --- Global Search ---

func (h *ToolHandlers) handleSearchGlobal(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	q := req.GetString("q", "")
	if q == "" {
		return mcp.NewToolResultError("q (search query) is required"), nil
	}

	params := redmine.GlobalSearchParams{
		Query:      q,
		Scope:      req.GetString("scope", ""),
		TitlesOnly: req.GetBool("titles_only", false),
		Issues:     req.GetBool("issues", false),
		WikiPages:  req.GetBool("wiki_pages", false),
		News:       req.GetBool("news", false),
		Documents:  req.GetBool("documents", false),
		Changesets: req.GetBool("changesets", false),
		Messages:   req.GetBool("messages", false),
		Projects:   req.GetBool("projects", false),
		Offset:     req.GetInt("offset", 0),
		Limit:      req.GetInt("limit", 25),
	}

	results, total, err := h.client.GlobalSearch(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search: %v", err)), nil
	}

	items := make([]map[string]any, len(results))
	for i, r := range results {
		item := map[string]any{
			"id":    r.ID,
			"title": r.Title,
			"type":  r.Type,
			"url":   r.URL,
		}
		if r.Description != "" {
			item["description"] = r.Description
		}
		if r.Datetime != "" {
			item["datetime"] = r.Datetime
		}
		items[i] = item
	}

	return jsonResult(map[string]any{
		"results":     items,
		"count":       len(results),
		"total_count": total,
	})
}

// --- Group B: Batch & Copy ---

func (h *ToolHandlers) handleIssuesBatchUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	issueIDsRaw := getArrayArg(req, "issue_ids")
	if len(issueIDsRaw) == 0 {
		return mcp.NewToolResultError("issue_ids is required and must be a non-empty array"), nil
	}

	// Parse issue IDs
	issueIDs := make([]int, 0, len(issueIDsRaw))
	for _, raw := range issueIDsRaw {
		if f, ok := raw.(float64); ok {
			issueIDs = append(issueIDs, int(f))
		} else {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid issue ID: %v", raw)), nil
		}
	}

	// Pre-resolve common fields once
	var statusID int
	if status := req.GetString("status", ""); status != "" {
		var err error
		statusID, err = h.resolver.ResolveStatusID(status)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve status: %v", err)), nil
		}
	}

	var priorityID int
	if priority := req.GetString("priority", ""); priority != "" {
		var err error
		priorityID, err = h.resolver.ResolvePriority(priority)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve priority: %v", err)), nil
		}
	}

	notes := req.GetString("notes", "")
	assignedToStr := req.GetString("assigned_to", "")

	// Process each issue
	var successIDs []int
	var failures []map[string]any

	for _, issueID := range issueIDs {
		params := redmine.UpdateIssueParams{
			IssueID:    issueID,
			StatusID:   statusID,
			PriorityID: priorityID,
			Notes:      notes,
		}

		// Resolve assigned_to per issue (needs project context)
		if assignedToStr != "" {
			issue, err := h.client.GetIssue(issueID)
			if err != nil {
				failures = append(failures, map[string]any{
					"id":    issueID,
					"error": fmt.Sprintf("Failed to get issue: %v", err),
				})
				continue
			}

			// Validate status transition if status is being changed
			if statusID > 0 && h.workflow != nil {
				if err := h.workflow.ValidateTransition(issue.Tracker.ID, issue.Status.ID, statusID); err != nil {
					failures = append(failures, map[string]any{
						"id":    issueID,
						"error": fmt.Sprintf("Invalid status transition: %v", err),
					})
					continue
				}
			}

			if assignedToStr == "me" {
				user, err := h.client.GetCurrentUser()
				if err != nil {
					failures = append(failures, map[string]any{
						"id":    issueID,
						"error": fmt.Sprintf("Failed to get current user: %v", err),
					})
					continue
				}
				params.AssignedToID = user.ID
			} else {
				userID, err := h.resolver.ResolveUser(assignedToStr, issue.Project.ID)
				if err != nil {
					failures = append(failures, map[string]any{
						"id":    issueID,
						"error": fmt.Sprintf("Failed to resolve assignee: %v", err),
					})
					continue
				}
				params.AssignedToID = userID
			}
		} else if statusID > 0 && h.workflow != nil {
			// Still need to validate status transition
			issue, err := h.client.GetIssue(issueID)
			if err != nil {
				failures = append(failures, map[string]any{
					"id":    issueID,
					"error": fmt.Sprintf("Failed to get issue: %v", err),
				})
				continue
			}
			if err := h.workflow.ValidateTransition(issue.Tracker.ID, issue.Status.ID, statusID); err != nil {
				failures = append(failures, map[string]any{
					"id":    issueID,
					"error": fmt.Sprintf("Invalid status transition: %v", err),
				})
				continue
			}
		}

		if err := h.client.UpdateIssue(params); err != nil {
			failures = append(failures, map[string]any{
				"id":    issueID,
				"error": fmt.Sprintf("Failed to update: %v", err),
			})
			continue
		}

		successIDs = append(successIDs, issueID)
	}

	return jsonResult(map[string]any{
		"success": successIDs,
		"failed":  failures,
	})
}

func (h *ToolHandlers) handleIssuesCopy(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	issueIDFloat, err := req.RequireFloat("issue_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	sourceID := int(issueIDFloat)

	// Get source issue
	source, err := h.client.GetIssue(sourceID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get source issue: %v", err)), nil
	}

	// Determine target project
	targetProjectID := source.Project.ID
	if project := req.GetString("project", ""); project != "" {
		targetProjectID, err = h.resolver.ResolveProject(project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve target project: %v", err)), nil
		}
	}

	// Build create params from source issue
	subject := source.Subject
	if overrideSubject := req.GetString("subject", ""); overrideSubject != "" {
		subject = overrideSubject
	}

	params := redmine.CreateIssueParams{
		ProjectID:   targetProjectID,
		TrackerID:   source.Tracker.ID,
		Subject:     subject,
		Description: source.Description,
		PriorityID:  source.Priority.ID,
		StartDate:   source.StartDate,
		DueDate:     source.DueDate,
	}

	if source.AssignedTo != nil {
		params.AssignedToID = source.AssignedTo.ID
	}

	// Copy custom fields
	if len(source.CustomFields) > 0 {
		cfMap := make(map[string]any)
		for _, cf := range source.CustomFields {
			if cf.Value != nil && cf.Value != "" {
				cfMap[strconv.Itoa(cf.ID)] = cf.Value
			}
		}
		if len(cfMap) > 0 {
			params.CustomFields = cfMap
		}
	}

	newIssue, err := h.client.CreateIssue(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create copy: %v", err)), nil
	}

	return jsonResult(formatIssue(*newIssue))
}

// --- Group C: Versions ---

func (h *ToolHandlers) handleVersionsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	versions, err := h.client.ListVersions(projectID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list versions: %v", err)), nil
	}

	results := make([]map[string]any, len(versions))
	for i, v := range versions {
		results[i] = map[string]any{
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

	return jsonResult(map[string]any{
		"versions": results,
		"count":    len(versions),
	})
}

func (h *ToolHandlers) handleVersionsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	params := redmine.CreateVersionParams{
		ProjectID:   projectID,
		Name:        name,
		Description: req.GetString("description", ""),
		Status:      req.GetString("status", ""),
		DueDate:     req.GetString("due_date", ""),
		Sharing:     req.GetString("sharing", ""),
	}

	version, err := h.client.CreateVersion(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create version: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"id":          version.ID,
		"name":        version.Name,
		"description": version.Description,
		"status":      version.Status,
		"due_date":    version.DueDate,
		"sharing":     version.Sharing,
		"created_on":  version.CreatedOn,
	})
}

func (h *ToolHandlers) handleVersionsUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("version_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	versionID := int(idFloat)

	params := redmine.UpdateVersionParams{
		VersionID:   versionID,
		Name:        req.GetString("name", ""),
		Description: req.GetString("description", ""),
		Status:      req.GetString("status", ""),
		DueDate:     req.GetString("due_date", ""),
		Sharing:     req.GetString("sharing", ""),
	}

	if err := h.client.UpdateVersion(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update version: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":    true,
		"version_id": versionID,
		"message":    "Version updated successfully",
	})
}

// --- Issue Categories ---

func (h *ToolHandlers) handleCategoriesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	categories, err := h.client.ListIssueCategories(projectID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list issue categories: %v", err)), nil
	}

	results := make([]map[string]any, len(categories))
	for i, c := range categories {
		result := map[string]any{
			"id":   c.ID,
			"name": c.Name,
		}
		if c.AssignedTo.ID > 0 {
			result["assigned_to"] = map[string]any{
				"id":   c.AssignedTo.ID,
				"name": c.AssignedTo.Name,
			}
		}
		results[i] = result
	}

	return jsonResult(map[string]any{
		"categories": results,
		"count":      len(categories),
	})
}

func (h *ToolHandlers) handleCategoriesCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	params := redmine.CreateIssueCategoryParams{
		ProjectID: projectID,
		Name:      name,
	}

	// Resolve assigned_to if provided
	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		userID, err := h.resolver.ResolveUser(assignedTo, projectID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve assigned_to: %v", err)), nil
		}
		params.AssignedToID = userID
	}

	category, err := h.client.CreateIssueCategory(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create issue category: %v", err)), nil
	}

	result := map[string]any{
		"id":   category.ID,
		"name": category.Name,
	}
	if category.AssignedTo.ID > 0 {
		result["assigned_to"] = map[string]any{
			"id":   category.AssignedTo.ID,
			"name": category.AssignedTo.Name,
		}
	}

	return jsonResult(result)
}

func (h *ToolHandlers) handleCategoriesUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("category_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	categoryID := int(idFloat)

	params := redmine.UpdateIssueCategoryParams{
		CategoryID: categoryID,
		Name:       req.GetString("name", ""),
	}

	// Resolve assigned_to if provided
	if assignedTo := req.GetString("assigned_to", ""); assignedTo != "" {
		// We don't have project context here, so pass 0 (resolver will search all projects)
		userID, err := h.resolver.ResolveUser(assignedTo, 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve assigned_to: %v", err)), nil
		}
		params.AssignedToID = userID
	}

	if err := h.client.UpdateIssueCategory(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update issue category: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":     true,
		"category_id": categoryID,
		"message":     "Category updated successfully",
	})
}

func (h *ToolHandlers) handleCategoriesDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("category_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	categoryID := int(idFloat)

	if err := h.client.DeleteIssueCategory(categoryID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete issue category: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":     true,
		"category_id": categoryID,
		"message":     "Category deleted successfully",
	})
}

// --- Project Memberships ---

func (h *ToolHandlers) handleMembershipsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	memberships, err := h.client.GetProjectMemberships(projectID, 1000)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list memberships: %v", err)), nil
	}

	result := make([]map[string]any, len(memberships))
	for i, m := range memberships {
		entry := map[string]any{
			"id":      m.ID,
			"project": map[string]any{"id": m.Project.ID, "name": m.Project.Name},
			"roles":   m.Roles,
		}
		if m.User != nil {
			entry["user"] = map[string]any{"id": m.User.ID, "name": m.User.Name}
		}
		if m.Group != nil {
			entry["group"] = map[string]any{"id": m.Group.ID, "name": m.Group.Name}
		}
		result[i] = entry
	}

	return jsonResult(map[string]any{
		"memberships": result,
		"count":       len(memberships),
	})
}

func (h *ToolHandlers) handleMembershipsAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	userStr := req.GetString("user", "")
	groupStr := req.GetString("group", "")

	if userStr == "" && groupStr == "" {
		return mcp.NewToolResultError("Must specify either user or group"), nil
	}
	if userStr != "" && groupStr != "" {
		return mcp.NewToolResultError("Cannot specify both user and group"), nil
	}

	// Resolve roles
	rolesArray := getArrayArg(req, "roles")
	if len(rolesArray) == 0 {
		return mcp.NewToolResultError("Must specify at least one role"), nil
	}

	roleIDs := make([]int, 0, len(rolesArray))
	for _, roleItem := range rolesArray {
		roleStr, ok := roleItem.(string)
		if !ok {
			return mcp.NewToolResultError("Role must be a string (name or ID)"), nil
		}
		roleID, err := h.resolver.ResolveRole(roleStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve role '%s': %v", roleStr, err)), nil
		}
		roleIDs = append(roleIDs, roleID)
	}

	var userID *int
	var groupID *int

	if userStr != "" {
		uid, err := h.resolver.ResolveUser(userStr, projectID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
		}
		userID = &uid
	}

	if groupStr != "" {
		// Groups are not resolved by name in current implementation - must be ID
		gid, err := strconv.Atoi(groupStr)
		if err != nil {
			return mcp.NewToolResultError("Group must be specified as numeric ID"), nil
		}
		groupID = &gid
	}

	membership, err := h.client.CreateProjectMembership(projectID, userID, groupID, roleIDs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create membership: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":       true,
		"membership_id": membership.ID,
		"membership":    membership,
		"message":       "Membership added successfully",
	})
}

func (h *ToolHandlers) handleMembershipsUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("membership_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	membershipID := int(idFloat)

	// Resolve roles
	rolesArray := getArrayArg(req, "roles")
	if len(rolesArray) == 0 {
		return mcp.NewToolResultError("Must specify at least one role"), nil
	}

	roleIDs := make([]int, 0, len(rolesArray))
	for _, roleItem := range rolesArray {
		roleStr, ok := roleItem.(string)
		if !ok {
			return mcp.NewToolResultError("Role must be a string (name or ID)"), nil
		}
		roleID, err := h.resolver.ResolveRole(roleStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve role '%s': %v", roleStr, err)), nil
		}
		roleIDs = append(roleIDs, roleID)
	}

	if err := h.client.UpdateProjectMembership(membershipID, roleIDs); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update membership: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":       true,
		"membership_id": membershipID,
		"message":       "Membership updated successfully",
	})
}

func (h *ToolHandlers) handleMembershipsRemove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idFloat, err := req.RequireFloat("membership_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	membershipID := int(idFloat)

	if err := h.client.DeleteProjectMembership(membershipID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to remove membership: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success":       true,
		"membership_id": membershipID,
		"message":       "Membership removed successfully",
	})
}

// --- Group D: Wiki ---

func (h *ToolHandlers) handleWikiList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	pages, err := h.client.ListWikiPages(projectID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list wiki pages: %v", err)), nil
	}

	results := make([]map[string]any, len(pages))
	for i, p := range pages {
		results[i] = map[string]any{
			"title":      p.Title,
			"version":    p.Version,
			"created_on": p.CreatedOn,
			"updated_on": p.UpdatedOn,
		}
	}

	return jsonResult(map[string]any{
		"wiki_pages": results,
		"count":      len(pages),
	})
}

func (h *ToolHandlers) handleWikiGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	page, err := h.client.GetWikiPage(projectID, title)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get wiki page: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"title":      page.Title,
		"text":       page.Text,
		"version":    page.Version,
		"author":     map[string]any{"id": page.Author.ID, "name": page.Author.Name},
		"comments":   page.Comments,
		"created_on": page.CreatedOn,
		"updated_on": page.UpdatedOn,
	})
}

func (h *ToolHandlers) handleWikiCreateOrUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkReadOnly(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	text, err := req.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	params := redmine.WikiPageParams{
		ProjectID: projectID,
		Title:     title,
		Text:      text,
		Comments:  req.GetString("comments", ""),
	}

	if err := h.client.CreateOrUpdateWikiPage(params); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create/update wiki page: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"success": true,
		"title":   title,
		"message": "Wiki page created/updated successfully",
	})
}

// --- Group F: Export ---

func (h *ToolHandlers) handleIssuesExportCSV(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Build search params (same logic as handleIssuesSearch)
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

	params.Subject = req.GetString("subject", "")
	params.Sort = req.GetString("sort", "")
	params.Limit = req.GetInt("limit", 25)
	params.Offset = req.GetInt("offset", 0)

	issues, _, err := h.client.SearchIssues(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search issues: %v", err)), nil
	}

	// Write CSV
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header
	_ = writer.Write([]string{"ID", "Subject", "Project", "Tracker", "Status", "Priority", "Assignee", "Created", "Updated"})

	for _, issue := range issues {
		assignee := ""
		if issue.AssignedTo != nil {
			assignee = issue.AssignedTo.Name
		}
		_ = writer.Write([]string{
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

	writer.Flush()
	if err := writer.Error(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write CSV: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.String()), nil
}

// --- Group G: Reports ---

func (h *ToolHandlers) handleReportsWeekly(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Resolve user
	userStr := req.GetString("user", "me")
	userID := "me"
	var userName string

	if userStr == "me" {
		user, err := h.client.GetCurrentUser()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get current user: %v", err)), nil
		}
		userName = user.Firstname + " " + user.Lastname
	} else {
		resolvedID, err := h.resolver.ResolveUser(userStr, 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
		}
		userID = strconv.Itoa(resolvedID)
		userName = userStr
	}

	// Calculate Monday-Friday of the target week
	now := time.Now()
	if weekOf := req.GetString("week_of", ""); weekOf != "" {
		parsed, err := time.Parse("2006-01-02", weekOf)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid date format for week_of: %v", err)), nil
		}
		now = parsed
	}

	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -weekday+1)
	friday := monday.AddDate(0, 0, 4)

	fromDate := monday.Format("2006-01-02")
	toDate := friday.Format("2006-01-02")

	// Fetch time entries for the week
	teParams := redmine.ListTimeEntriesParams{
		UserID: userID,
		From:   fromDate,
		To:     toDate,
		Limit:  100,
	}

	var allEntries []redmine.TimeEntry
	for {
		entries, _, err := h.client.ListTimeEntries(teParams)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch time entries: %v", err)), nil
		}
		allEntries = append(allEntries, entries...)
		if len(entries) < teParams.Limit {
			break
		}
		teParams.Offset += teParams.Limit
	}

	// Aggregate by day
	byDay := make(map[string]float64)
	// Initialize all weekdays
	for i := 0; i < 5; i++ {
		day := monday.AddDate(0, 0, i).Format("2006-01-02")
		byDay[day] = 0
	}

	// Aggregate by issue
	type issueAgg struct {
		Subject string
		Hours   float64
	}
	byIssue := make(map[int]*issueAgg)

	// Aggregate by activity
	byActivity := make(map[string]float64)

	var totalHours float64
	for _, entry := range allEntries {
		byDay[entry.SpentOn] += entry.Hours
		totalHours += entry.Hours

		if entry.Issue != nil {
			if agg, ok := byIssue[entry.Issue.ID]; ok {
				agg.Hours += entry.Hours
			} else {
				byIssue[entry.Issue.ID] = &issueAgg{
					Subject: entry.Issue.Name,
					Hours:   entry.Hours,
				}
			}
		}

		byActivity[entry.Activity.Name] += entry.Hours
	}

	// Build by_day result (sorted)
	dayKeys := make([]string, 0, len(byDay))
	for k := range byDay {
		dayKeys = append(dayKeys, k)
	}
	sort.Strings(dayKeys)

	byDayResult := make([]map[string]any, len(dayKeys))
	for i, date := range dayKeys {
		byDayResult[i] = map[string]any{
			"date":  date,
			"hours": byDay[date],
		}
	}

	// Build by_issue result
	byIssueResult := make([]map[string]any, 0, len(byIssue))
	for id, agg := range byIssue {
		byIssueResult = append(byIssueResult, map[string]any{
			"issue_id": id,
			"subject":  agg.Subject,
			"hours":    agg.Hours,
		})
	}
	sort.Slice(byIssueResult, func(i, j int) bool {
		return byIssueResult[i]["hours"].(float64) > byIssueResult[j]["hours"].(float64)
	})

	// Build by_activity result
	byActivityResult := make([]map[string]any, 0, len(byActivity))
	for name, hours := range byActivity {
		byActivityResult = append(byActivityResult, map[string]any{
			"activity": name,
			"hours":    hours,
		})
	}
	sort.Slice(byActivityResult, func(i, j int) bool {
		return byActivityResult[i]["hours"].(float64) > byActivityResult[j]["hours"].(float64)
	})

	return jsonResult(map[string]any{
		"user":        userName,
		"period":      fmt.Sprintf("%s ~ %s", fromDate, toDate),
		"total_hours": totalHours,
		"by_day":      byDayResult,
		"by_issue":    byIssueResult,
		"by_activity": byActivityResult,
	})
}

func (h *ToolHandlers) handleReportsStandup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Resolve user
	userStr := req.GetString("user", "me")
	var userID string
	var userName string
	var assignedToID string

	if userStr == "me" {
		user, err := h.client.GetCurrentUser()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get current user: %v", err)), nil
		}
		userID = "me"
		assignedToID = "me"
		userName = user.Firstname + " " + user.Lastname
	} else {
		resolvedID, err := h.resolver.ResolveUser(userStr, 0)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve user: %v", err)), nil
		}
		userID = strconv.Itoa(resolvedID)
		assignedToID = userID
		userName = userStr
	}

	// Determine dates
	today := time.Now()
	if dateStr := req.GetString("date", ""); dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid date format: %v", err)), nil
		}
		today = parsed
	}

	todayStr := today.Format("2006-01-02")

	// Calculate yesterday (skip weekends)
	yesterday := today.AddDate(0, 0, -1)
	if yesterday.Weekday() == time.Sunday {
		yesterday = yesterday.AddDate(0, 0, -2) // Friday
	} else if yesterday.Weekday() == time.Saturday {
		yesterday = yesterday.AddDate(0, 0, -1) // Friday
	}
	yesterdayStr := yesterday.Format("2006-01-02")

	// Fetch yesterday's time entries
	teParams := redmine.ListTimeEntriesParams{
		UserID: userID,
		From:   yesterdayStr,
		To:     yesterdayStr,
		Limit:  100,
	}

	entries, _, err := h.client.ListTimeEntries(teParams)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch time entries: %v", err)), nil
	}

	var yesterdayTotal float64
	yesterdayEntries := make([]map[string]any, len(entries))
	for i, entry := range entries {
		yesterdayTotal += entry.Hours
		e := map[string]any{
			"hours":    entry.Hours,
			"activity": entry.Activity.Name,
		}
		if entry.Issue != nil {
			e["issue_id"] = entry.Issue.ID
			e["subject"] = entry.Issue.Name
		}
		yesterdayEntries[i] = e
	}

	// Fetch today's open issues assigned to user
	searchParams := redmine.SearchIssuesParams{
		AssignedToID: assignedToID,
		StatusID:     "open",
		Limit:        50,
		Sort:         "priority:desc",
	}

	issues, _, err := h.client.SearchIssues(searchParams)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch open issues: %v", err)), nil
	}

	openIssues := make([]map[string]any, len(issues))
	for i, issue := range issues {
		openIssues[i] = map[string]any{
			"id":       issue.ID,
			"subject":  issue.Subject,
			"status":   issue.Status.Name,
			"priority": issue.Priority.Name,
		}
	}

	return jsonResult(map[string]any{
		"user": userName,
		"date": todayStr,
		"yesterday": map[string]any{
			"date":        yesterdayStr,
			"total_hours": yesterdayTotal,
			"entries":     yesterdayEntries,
		},
		"today": map[string]any{
			"open_issues": openIssues,
		},
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

	// Fallback: use custom field rules for name-to-ID mapping
	if h.rules != nil {
		for idStr, field := range h.rules.Fields {
			if id, err := strconv.Atoi(idStr); err == nil {
				nameToID[strings.ToLower(field.Name)] = id
			}
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

	// Check for missing required fields (per-tracker)
	if h.rules != nil && trackerID > 0 {
		var missing []string
		for idStr, rule := range h.rules.Fields {
			if !slices.Contains(rule.RequiredByTrackers, trackerID) {
				continue
			}
			if _, exists := result[idStr]; !exists {
				missing = append(missing, fmt.Sprintf("%s (ID: %s, values: %s)",
					rule.Name, idStr, strings.Join(rule.Values, ", ")))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return nil, fmt.Errorf("required custom field(s) missing: %s", strings.Join(missing, "; "))
		}
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

	// Attachments
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
		// Try direct map type
		if m, ok := v.(map[string]any); ok {
			return m
		}
		// Try parsing from JSON string (MCP sometimes stringifies objects)
		if s, ok := v.(string); ok && strings.HasPrefix(s, "{") {
			var m map[string]any
			if err := json.Unmarshal([]byte(s), &m); err == nil {
				return m
			}
		}
	}
	return nil
}

func getArrayArg(req mcp.CallToolRequest, key string) []any {
	args := req.GetArguments()
	if v, ok := args[key]; ok {
		// Try direct array type
		if arr, ok := v.([]any); ok {
			return arr
		}
		// Try parsing from JSON string (MCP sometimes stringifies arrays)
		if s, ok := v.(string); ok && strings.HasPrefix(s, "[") {
			var arr []any
			if err := json.Unmarshal([]byte(s), &arr); err == nil {
				return arr
			}
		}
	}
	return nil
}

func parseUploadTokens(tokens []any) ([]redmine.UploadToken, error) {
	uploads := make([]redmine.UploadToken, 0, len(tokens))
	for _, t := range tokens {
		m, ok := t.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("upload_tokens items must be objects with 'token' and 'filename' fields")
		}
		token, _ := m["token"].(string)
		filename, _ := m["filename"].(string)
		if token == "" || filename == "" {
			return nil, fmt.Errorf("upload_tokens items require 'token' and 'filename' fields")
		}
		u := redmine.UploadToken{
			Token:    token,
			Filename: filename,
		}
		if ct, ok := m["content_type"].(string); ok {
			u.ContentType = ct
		}
		if desc, ok := m["description"].(string); ok {
			u.Description = desc
		}
		uploads = append(uploads, u)
	}
	return uploads, nil
}

func (h *ToolHandlers) handleReportsProjectAnalysis(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectStr, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	projectID, err := h.resolver.ResolveProject(projectStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve project: %v", err)), nil
	}

	// Get project details
	project, err := h.client.GetProjectDetail(projectID, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get project: %v", err)), nil
	}

	// Parse custom_fields parameter
	var customFields []string
	if cfStr := req.GetString("custom_fields", ""); cfStr != "" {
		for _, cf := range strings.Split(cfStr, ",") {
			customFields = append(customFields, strings.TrimSpace(cf))
		}
	}

	params := ProjectAnalysisParams{
		ProjectID:    projectID,
		ProjectName:  project.Name,
		From:         req.GetString("from", ""),
		To:           req.GetString("to", ""),
		IssueStatus:  req.GetString("issue_status", "all"),
		Version:      req.GetString("version", ""),
		CustomFields: customFields,
		Format:       req.GetString("format", "json"),
		AttachTo:     req.GetString("attach_to", ""),
	}

	// Generate report
	rg := NewReportGenerator(h.client, h.resolver)
	result, err := rg.GenerateProjectAnalysis(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to generate analysis: %v", err)), nil
	}

	// Handle output format
	switch params.Format {
	case "csv":
		csv := rg.GenerateCSV(result)
		if params.AttachTo != "" {
			filename := fmt.Sprintf("%s_analysis_%s.csv", project.Identifier, time.Now().Format("20060102"))
			url, err := rg.AttachResult([]byte(csv), filename, params)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to attach report: %v", err)), nil
			}
			result.DownloadURL = url
			return jsonResult(result)
		}
		return mcp.NewToolResultText(csv), nil

	default: // json
		return jsonResult(result)
	}
}


