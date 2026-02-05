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

		// Issues
		r.Get("/issues", s.handleSearchIssues)
		r.Get("/issues/{id}", s.handleGetIssue)
		r.Post("/issues", s.handleCreateIssue)
		r.Patch("/issues/{id}", s.handleUpdateIssue)
		r.Post("/issues/{id}/subtasks", s.handleCreateSubtask)
		r.Post("/issues/{id}/watchers", s.handleAddWatcher)
		r.Post("/issues/{id}/relations", s.handleAddRelation)

		// Time entries
		r.Post("/time_entries", s.handleCreateTimeEntry)

		// Custom Fields
		r.Get("/custom_fields", s.handleListAllCustomFields)

		// Reference
		r.Get("/trackers", s.handleListTrackers)
		r.Get("/statuses", s.handleListStatuses)
		r.Get("/activities", s.handleListActivities)
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
