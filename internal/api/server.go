package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"github.com/ycho/redmine-mcp-server/internal/redmine"

	_ "github.com/ycho/redmine-mcp-server/docs" // swagger docs
)

// Config holds API server configuration
type Config struct {
	RedmineURL           string
	Port                 int
	CustomFieldRulesFile string
	WorkflowRulesFile    string
}

// Server is the REST API server
type Server struct {
	config      Config
	router      *chi.Mux
	rateLimiter *RateLimiter
	rules       *redmine.CustomFieldRules
	workflow    *redmine.WorkflowRules
}

// NewServer creates a new API server
func NewServer(config Config) *Server {
	var rules *redmine.CustomFieldRules
	if config.CustomFieldRulesFile != "" {
		var err error
		rules, err = redmine.LoadCustomFieldRules(config.CustomFieldRulesFile)
		if err != nil {
			slog.Warn("Failed to load custom field rules", "file", config.CustomFieldRulesFile, "error", err)
		} else if rules != nil {
			slog.Info("Loaded custom field rules", "file", config.CustomFieldRulesFile, "fields", len(rules.Fields))
		}
	}

	var workflow *redmine.WorkflowRules
	if config.WorkflowRulesFile != "" {
		var werr error
		workflow, werr = redmine.LoadWorkflowRules(config.WorkflowRulesFile)
		if werr != nil {
			slog.Warn("Failed to load workflow rules", "file", config.WorkflowRulesFile, "error", werr)
		} else if workflow != nil {
			slog.Info("Loaded workflow rules", "file", config.WorkflowRulesFile, "trackers", len(workflow.Trackers))
		}
	}

	s := &Server{
		config:      config,
		router:      chi.NewRouter(),
		rateLimiter: NewRateLimiter(100, time.Second, 200), // 100 req/sec, burst 200
		rules:       rules,
		workflow:    workflow,
	}

	s.setupRoutes()

	// Start rate limiter cleanup goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.rateLimiter.Cleanup(10 * time.Minute)
		}
	}()

	return s
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	r := s.router

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)            // Security headers
	r.Use(s.rateLimiter.Middleware)   // Rate limiting

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Swagger UI - uses swaggo generated docs
	r.Get("/docs/*", httpSwagger.Handler(
		httpSwagger.URL("/docs/doc.json"),
	))

	// OpenAPI spec (static inline)
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(openAPISpec))
	})

	// API routes with authentication middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authMiddleware)

		// Account
		r.Get("/me", s.handleMe)

		// Projects
		r.Get("/projects", s.handleListProjects)
		r.Post("/projects", s.handleCreateProject)
		r.Get("/projects/{id}", s.handleGetProject)
		r.Patch("/projects/{id}", s.handleUpdateProject)

		// Issues (note: export.csv and batch-update must come before {id} to avoid wildcard match)
		r.Get("/issues/export.csv", s.handleExportIssuesCSV)
		r.Post("/issues/batch-update", s.handleBatchUpdateIssues)
		r.Get("/issues", s.handleSearchIssues)
		r.Get("/issues/{id}", s.handleGetIssue)
		r.Post("/issues", s.handleCreateIssue)
		r.Patch("/issues/{id}", s.handleUpdateIssue)
		r.Post("/issues/{id}/subtasks", s.handleCreateSubtask)
		r.Post("/issues/{id}/watchers", s.handleAddWatcher)
		r.Delete("/issues/{id}/watchers/{user_id}", s.handleRemoveWatcher)
		r.Post("/issues/{id}/relations", s.handleAddRelation)
		r.Post("/issues/{id}/copy", s.handleCopyIssue)

		// Relations
		r.Delete("/relations/{id}", s.handleDeleteRelation)

		// Time entries
		r.Post("/time_entries", s.handleCreateTimeEntry)
		r.Patch("/time_entries/{id}", s.handleUpdateTimeEntry)
		r.Delete("/time_entries/{id}", s.handleDeleteTimeEntry)

		// Users
		r.Get("/users", s.handleSearchUsers)

		// Search
		r.Get("/search", s.handleGlobalSearch)

		// Versions
		r.Get("/projects/{id}/versions", s.handleListVersions)
		r.Post("/projects/{id}/versions", s.handleCreateVersion)
		r.Patch("/versions/{id}", s.handleUpdateVersion)

		// Wiki
		r.Get("/projects/{id}/wiki", s.handleListWikiPages)
		r.Get("/projects/{id}/wiki/{title}", s.handleGetWikiPage)
		r.Put("/projects/{id}/wiki/{title}", s.handleCreateOrUpdateWikiPage)

		// Attachments
		r.Post("/attachments/upload", s.handleUploadAttachment)
		r.Get("/attachments/{id}/download", s.handleDownloadAttachment)
		r.Get("/issues/{id}/attachments", s.handleListAttachments)
		r.Post("/issues/{id}/attach", s.handleAttachToIssue)

		// Custom Fields
		r.Get("/custom_fields", s.handleListAllCustomFields)

		// Reference
		r.Get("/trackers", s.handleListTrackers)
		r.Get("/statuses", s.handleListStatuses)
		r.Get("/activities", s.handleListActivities)

		// Reports
		r.Get("/reports/weekly", s.handleWeeklyReport)
		r.Get("/reports/standup", s.handleStandupReport)
	})
}

// authMiddleware extracts the Redmine API key and creates a client
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-Redmine-API-Key")
		if apiKey == "" {
			http.Error(w, `{"error": "Missing X-Redmine-API-Key header"}`, http.StatusUnauthorized)
			return
		}

		// Create client and store in context
		client := redmine.NewClient(s.config.RedmineURL, apiKey)
		ctx := withClient(r.Context(), client)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Run starts the API server
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	slog.Info("Starting REST API server",
		"address", addr,
		"redmine_url", s.config.RedmineURL,
		"docs", fmt.Sprintf("http://localhost:%d/docs/index.html", s.config.Port),
	)

	return http.ListenAndServe(addr, s.router)
}

const openAPISpec = `openapi: 3.0.3
info:
  title: Redmine MCP Server API
  description: REST API for Redmine integration with AI assistants
  version: 1.0.0
servers:
  - url: /api/v1
security:
  - ApiKeyAuth: []
components:
  securitySchemes:
    ApiKeyAuth:
      type: apiKey
      in: header
      name: X-Redmine-API-Key
  schemas:
    Error:
      type: object
      properties:
        error:
          type: string
paths:
  /me:
    get:
      summary: Get current user information
      tags: [Account]
      responses:
        '200':
          description: Current user
  /projects:
    get:
      summary: List projects
      tags: [Projects]
      parameters:
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
      responses:
        '200':
          description: List of projects
    post:
      summary: Create a project
      tags: [Projects]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [name, identifier]
              properties:
                name:
                  type: string
                identifier:
                  type: string
                description:
                  type: string
                parent:
                  type: string
      responses:
        '201':
          description: Created project
  /projects/{id}:
    get:
      summary: Get project details
      tags: [Projects]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
      responses:
        '200':
          description: Project details with trackers and custom fields
    patch:
      summary: Update project settings
      tags: [Projects]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                description:
                  type: string
                tracker_ids:
                  type: array
                  items:
                    type: string
                  description: Tracker names or IDs to enable
                issue_custom_field_ids:
                  type: array
                  items:
                    type: string
                  description: Custom field names or IDs to enable
      responses:
        '200':
          description: Project updated
  /custom_fields:
    get:
      summary: List all custom fields (admin)
      tags: [Custom Fields]
      parameters:
        - name: type
          in: query
          schema:
            type: string
          description: "Filter by type: issue, project, user, time_entry, version, group"
      responses:
        '200':
          description: List of custom field definitions
  /issues:
    get:
      summary: Search issues
      tags: [Issues]
      parameters:
        - name: project
          in: query
          schema:
            type: string
          description: Project name or ID
        - name: tracker
          in: query
          schema:
            type: string
          description: Tracker name or ID
        - name: status
          in: query
          schema:
            type: string
          description: "Status: open, closed, all, or specific name"
        - name: assigned_to
          in: query
          schema:
            type: string
          description: "Assignee name or 'me'"
        - name: limit
          in: query
          schema:
            type: integer
            default: 25
      responses:
        '200':
          description: List of issues
    post:
      summary: Create an issue
      tags: [Issues]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [project, tracker, subject]
              properties:
                project:
                  type: string
                  description: Project name or ID
                tracker:
                  type: string
                  description: Tracker name or ID
                subject:
                  type: string
                description:
                  type: string
                assigned_to:
                  type: string
                  description: Assignee name or ID
                parent_issue_id:
                  type: integer
                start_date:
                  type: string
                  format: date
                due_date:
                  type: string
                  format: date
                custom_fields:
                  type: object
                  description: Custom fields as key-value pairs (field name -> value)
      responses:
        '201':
          description: Created issue
  /issues/{id}:
    get:
      summary: Get issue details
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
      responses:
        '200':
          description: Issue details with journals and relations
    patch:
      summary: Update an issue
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                status:
                  type: string
                  description: Status name or ID
                assigned_to:
                  type: string
                  description: Assignee name or ID
                notes:
                  type: string
                  description: Comment to add
                custom_fields:
                  type: object
      responses:
        '200':
          description: Issue updated
  /issues/{id}/subtasks:
    post:
      summary: Create a subtask
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Parent issue ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [subject]
              properties:
                subject:
                  type: string
                tracker:
                  type: string
                description:
                  type: string
                assigned_to:
                  type: string
                custom_fields:
                  type: object
      responses:
        '201':
          description: Created subtask
  /issues/{id}/watchers:
    post:
      summary: Add a watcher
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [user]
              properties:
                user:
                  type: string
                  description: User name or ID
      responses:
        '200':
          description: Watcher added
  /issues/{id}/relations:
    post:
      summary: Create a relation
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Source issue ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [issue_to_id, relation_type]
              properties:
                issue_to_id:
                  type: integer
                  description: Target issue ID
                relation_type:
                  type: string
                  enum: [relates, duplicates, blocks, precedes, copied_to]
      responses:
        '201':
          description: Relation created
  /time_entries:
    post:
      summary: Create a time entry
      tags: [Time Entries]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [issue_id, hours]
              properties:
                issue_id:
                  type: integer
                hours:
                  type: number
                activity:
                  type: string
                  description: Activity name or ID
                comments:
                  type: string
      responses:
        '201':
          description: Time entry created
  /attachments/upload:
    post:
      summary: Upload a file
      tags: [Attachments]
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
              required: [file]
              properties:
                file:
                  type: string
                  format: binary
                  description: File to upload (max 5MB)
      responses:
        '200':
          description: Upload token
  /attachments/{id}/download:
    get:
      summary: Download an attachment
      tags: [Attachments]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Attachment ID
      responses:
        '200':
          description: File content
          content:
            application/octet-stream:
              schema:
                type: string
                format: binary
  /issues/{id}/attachments:
    get:
      summary: List issue attachments
      tags: [Attachments]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Issue ID
      responses:
        '200':
          description: List of attachments
  /issues/{id}/attach:
    post:
      summary: Attach files to an issue
      tags: [Attachments]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Issue ID
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
              required: ["files[]"]
              properties:
                "files[]":
                  type: array
                  items:
                    type: string
                    format: binary
                  description: Files to attach (max 5MB each)
                notes:
                  type: string
                  description: Notes/comment to add
      responses:
        '200':
          description: Files attached
  /time_entries/{id}:
    patch:
      summary: Update a time entry
      tags: [Time Entries]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Time Entry ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                hours:
                  type: number
                activity:
                  type: string
                  description: Activity name or ID
                comments:
                  type: string
                spent_on:
                  type: string
                  format: date
      responses:
        '200':
          description: Time entry updated
    delete:
      summary: Delete a time entry
      tags: [Time Entries]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Time Entry ID
      responses:
        '200':
          description: Time entry deleted
  /issues/{id}/watchers/{user_id}:
    delete:
      summary: Remove a watcher
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Issue ID
        - name: user_id
          in: path
          required: true
          schema:
            type: integer
          description: User ID
      responses:
        '200':
          description: Watcher removed
  /relations/{id}:
    delete:
      summary: Delete a relation
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Relation ID
      responses:
        '200':
          description: Relation deleted
  /users:
    get:
      summary: Search users
      tags: [Users]
      parameters:
        - name: name
          in: query
          schema:
            type: string
          description: Filter by user name
        - name: project
          in: query
          schema:
            type: string
          description: Project name or ID for context
        - name: status
          in: query
          schema:
            type: integer
          description: "User status: 1=active, 2=registered, 3=locked"
        - name: limit
          in: query
          schema:
            type: integer
            default: 25
      responses:
        '200':
          description: List of users
  /issues/batch-update:
    post:
      summary: Batch update issues
      tags: [Issues]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [issue_ids]
              properties:
                issue_ids:
                  type: array
                  items:
                    type: integer
                  description: List of issue IDs to update
                status:
                  type: string
                  description: Status name or ID
                assigned_to:
                  type: string
                  description: Assignee name or ID
                priority:
                  type: string
                  description: Priority name or ID
                notes:
                  type: string
                  description: Comment to add
      responses:
        '200':
          description: Batch update results with success and failed arrays
  /issues/{id}/copy:
    post:
      summary: Copy an issue
      tags: [Issues]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Source issue ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                project:
                  type: string
                  description: Target project name or ID (defaults to source project)
                subject:
                  type: string
                  description: New subject (defaults to source subject)
      responses:
        '201':
          description: Copied issue
  /projects/{id}/versions:
    get:
      summary: List project versions
      tags: [Versions]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
      responses:
        '200':
          description: List of versions
    post:
      summary: Create a version
      tags: [Versions]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
                description:
                  type: string
                status:
                  type: string
                  enum: [open, locked, closed]
                due_date:
                  type: string
                  format: date
                sharing:
                  type: string
                  enum: [none, descendants, hierarchy, tree, system]
      responses:
        '201':
          description: Created version
  /versions/{id}:
    patch:
      summary: Update a version
      tags: [Versions]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Version ID
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                description:
                  type: string
                status:
                  type: string
                  enum: [open, locked, closed]
                due_date:
                  type: string
                  format: date
                sharing:
                  type: string
                  enum: [none, descendants, hierarchy, tree, system]
      responses:
        '200':
          description: Version updated
  /projects/{id}/wiki:
    get:
      summary: List wiki pages
      tags: [Wiki]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
      responses:
        '200':
          description: List of wiki pages
  /projects/{id}/wiki/{title}:
    get:
      summary: Get wiki page
      tags: [Wiki]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
        - name: title
          in: path
          required: true
          schema:
            type: string
          description: Wiki page title
      responses:
        '200':
          description: Wiki page with content
    put:
      summary: Create or update wiki page
      tags: [Wiki]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          description: Project ID
        - name: title
          in: path
          required: true
          schema:
            type: string
          description: Wiki page title
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [text]
              properties:
                text:
                  type: string
                  description: Wiki page content (Textile markup)
                comments:
                  type: string
                  description: Edit comment / version note
      responses:
        '200':
          description: Wiki page saved
  /issues/export.csv:
    get:
      summary: Export issues as CSV
      tags: [Issues]
      parameters:
        - name: project
          in: query
          schema:
            type: string
          description: Project name or ID
        - name: tracker
          in: query
          schema:
            type: string
          description: Tracker name or ID
        - name: status
          in: query
          schema:
            type: string
          description: "Status: open, closed, all, or specific name"
        - name: assigned_to
          in: query
          schema:
            type: string
          description: "Assignee name or 'me'"
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
      responses:
        '200':
          description: CSV file
          content:
            text/csv:
              schema:
                type: string
  /search:
    get:
      summary: Search across all Redmine resources
      tags: [Search]
      parameters:
        - name: q
          in: query
          required: true
          schema:
            type: string
          description: Search query
        - name: scope
          in: query
          schema:
            type: string
          description: "Search scope: all, my_projects, subprojects"
        - name: titles_only
          in: query
          schema:
            type: boolean
          description: Match only in titles
        - name: issues
          in: query
          schema:
            type: boolean
          description: Include issues
        - name: wiki_pages
          in: query
          schema:
            type: boolean
          description: Include wiki pages
        - name: limit
          in: query
          schema:
            type: integer
            default: 25
      responses:
        '200':
          description: Search results
  /reports/weekly:
    get:
      summary: Weekly time report
      tags: [Reports]
      parameters:
        - name: user
          in: query
          schema:
            type: string
            default: me
          description: "User name or 'me'"
        - name: week_of
          in: query
          schema:
            type: string
            format: date
          description: Date within the target week (YYYY-MM-DD)
      responses:
        '200':
          description: Weekly report with daily totals and entries
  /reports/standup:
    get:
      summary: Standup report
      tags: [Reports]
      parameters:
        - name: user
          in: query
          schema:
            type: string
            default: me
          description: "User name or 'me'"
        - name: date
          in: query
          schema:
            type: string
            format: date
          description: Report date (YYYY-MM-DD), defaults to today
      responses:
        '200':
          description: Standup report with yesterday's work and today's open issues
  /trackers:
    get:
      summary: List trackers
      tags: [Reference]
      responses:
        '200':
          description: List of trackers
  /statuses:
    get:
      summary: List issue statuses
      tags: [Reference]
      responses:
        '200':
          description: List of statuses
  /activities:
    get:
      summary: List time entry activities
      tags: [Reference]
      responses:
        '200':
          description: List of activities
`
