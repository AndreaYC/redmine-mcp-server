package redmine

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// UpdateTimeEntry
// ---------------------------------------------------------------------------

func TestUpdateTimeEntry_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /time_entries/1.json", func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header
		if r.Header.Get("X-Redmine-API-Key") != "test-key" {
			t.Errorf("expected API key header 'test-key', got %q", r.Header.Get("X-Redmine-API-Key"))
		}

		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		te, ok := req["time_entry"]
		if !ok {
			t.Fatal("request body missing 'time_entry' key")
		}
		if hours, ok := te["hours"].(float64); !ok || hours != 2.5 {
			t.Errorf("expected hours=2.5, got %v", te["hours"])
		}

		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateTimeEntry(UpdateTimeEntryParams{TimeEntryID: 1, Hours: 2.5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateTimeEntry_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /time_entries/999.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Not found"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateTimeEntry(UpdateTimeEntryParams{TimeEntryID: 999, Hours: 1.0})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

func TestUpdateTimeEntry_InvalidParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /time_entries/1.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":["Hours is invalid"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateTimeEntry(UpdateTimeEntryParams{TimeEntryID: 1, Hours: -1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected error to contain '422', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteTimeEntry
// ---------------------------------------------------------------------------

func TestDeleteTimeEntry_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /time_entries/1.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.DeleteTimeEntry(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteTimeEntry_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /time_entries/999.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Not found"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.DeleteTimeEntry(999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RemoveWatcher
// ---------------------------------------------------------------------------

func TestRemoveWatcher_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /issues/1/watchers/2.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.RemoveWatcher(1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveWatcher_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /issues/1/watchers/999.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Not found"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.RemoveWatcher(1, 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteRelation
// ---------------------------------------------------------------------------

func TestDeleteRelation_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /relations/1.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.DeleteRelation(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteRelation_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /relations/999.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Not found"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.DeleteRelation(999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SearchUsers
// ---------------------------------------------------------------------------

func TestSearchUsers_Admin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"users": [
				{"id":1,"login":"john","firstname":"John","lastname":"Doe","mail":"john@example.com"},
				{"id":2,"login":"jane","firstname":"Jane","lastname":"Smith","mail":"jane@example.com"}
			],
			"total_count": 2
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	users, total, err := client.SearchUsers(SearchUsersParams{Limit: 25})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total_count=2, got %d", total)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].ID != 1 {
		t.Errorf("expected first user ID=1, got %d", users[0].ID)
	}
	if users[0].Login != "john" {
		t.Errorf("expected first user login='john', got %q", users[0].Login)
	}
	// Name should be auto-filled from Firstname + Lastname
	if users[0].Name != "John Doe" {
		t.Errorf("expected first user name='John Doe', got %q", users[0].Name)
	}
}

func TestSearchUsers_Fallback(t *testing.T) {
	mux := http.NewServeMux()
	// Admin API returns 403
	mux.HandleFunc("GET /users.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	})
	// Memberships fallback
	mux.HandleFunc("GET /projects/1/memberships.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"memberships": [
				{
					"id": 10,
					"project": {"id":1,"name":"Test Project"},
					"user": {"id":1,"name":"John Doe"},
					"roles": [{"id":3,"name":"Manager"}]
				},
				{
					"id": 11,
					"project": {"id":1,"name":"Test Project"},
					"user": {"id":2,"name":"Jane Smith"},
					"roles": [{"id":4,"name":"Developer"}]
				},
				{
					"id": 12,
					"project": {"id":1,"name":"Test Project"},
					"group": {"id":100,"name":"Team Alpha"},
					"roles": [{"id":4,"name":"Developer"}]
				}
			],
			"total_count": 3
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	users, total, err := client.SearchUsers(SearchUsersParams{ProjectID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Group entries should be skipped, only user entries returned
	if len(users) != 2 {
		t.Fatalf("expected 2 users (groups filtered), got %d", len(users))
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if users[0].Name != "John Doe" {
		t.Errorf("expected first user name='John Doe', got %q", users[0].Name)
	}
}

func TestSearchUsers_NoResults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[],"total_count":0}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	users, total, err := client.SearchUsers(SearchUsersParams{Name: "nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestSearchUsers_FallbackNameFilter(t *testing.T) {
	mux := http.NewServeMux()
	// Admin API returns 403
	mux.HandleFunc("GET /users.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	})
	// Memberships fallback with multiple users
	mux.HandleFunc("GET /projects/1/memberships.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"memberships": [
				{
					"id": 10,
					"project": {"id":1,"name":"Test Project"},
					"user": {"id":1,"name":"John Doe"},
					"roles": [{"id":3,"name":"Manager"}]
				},
				{
					"id": 11,
					"project": {"id":1,"name":"Test Project"},
					"user": {"id":2,"name":"Jane Smith"},
					"roles": [{"id":4,"name":"Developer"}]
				},
				{
					"id": 12,
					"project": {"id":1,"name":"Test Project"},
					"user": {"id":3,"name":"Bob Johnson"},
					"roles": [{"id":4,"name":"Developer"}]
				}
			],
			"total_count": 3
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	// Filter by name "jane" (case-insensitive)
	users, total, err := client.SearchUsers(SearchUsersParams{ProjectID: 1, Name: "jane"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user matching 'jane', got %d", len(users))
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
	if users[0].Name != "Jane Smith" {
		t.Errorf("expected user name='Jane Smith', got %q", users[0].Name)
	}
}

func TestSearchUsers_NoProjectFallbackError(t *testing.T) {
	mux := http.NewServeMux()
	// Admin API returns 403
	mux.HandleFunc("GET /users.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	// No project ID provided, so fallback cannot work
	_, _, err := client.SearchUsers(SearchUsersParams{Name: "john"})
	if err == nil {
		t.Fatal("expected error when admin API fails and no project ID provided")
	}
	if !strings.Contains(err.Error(), "admin privileges") {
		t.Errorf("expected error about admin privileges, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListVersions
// ---------------------------------------------------------------------------

func TestListVersions_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"versions": [
				{
					"id": 1,
					"project": {"id":1,"name":"Test Project"},
					"name": "v1.0",
					"description": "First release",
					"status": "open",
					"due_date": "2025-06-01",
					"sharing": "none",
					"created_on": "2025-01-01T00:00:00Z",
					"updated_on": "2025-01-01T00:00:00Z"
				},
				{
					"id": 2,
					"project": {"id":1,"name":"Test Project"},
					"name": "v2.0",
					"description": "Second release",
					"status": "locked",
					"due_date": "2025-12-01",
					"sharing": "descendants",
					"created_on": "2025-02-01T00:00:00Z",
					"updated_on": "2025-02-01T00:00:00Z"
				}
			]
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	versions, err := client.ListVersions(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Name != "v1.0" {
		t.Errorf("expected first version name='v1.0', got %q", versions[0].Name)
	}
	if versions[0].Status != "open" {
		t.Errorf("expected first version status='open', got %q", versions[0].Status)
	}
	if versions[1].Name != "v2.0" {
		t.Errorf("expected second version name='v2.0', got %q", versions[1].Name)
	}
	if versions[1].Sharing != "descendants" {
		t.Errorf("expected second version sharing='descendants', got %q", versions[1].Sharing)
	}
}

func TestListVersions_Empty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versions":[]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	versions, err := client.ListVersions(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

// ---------------------------------------------------------------------------
// CreateVersion
// ---------------------------------------------------------------------------

func TestCreateVersion_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects/1/versions.json", func(w http.ResponseWriter, r *http.Request) {
		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		v, ok := req["version"]
		if !ok {
			t.Fatal("request body missing 'version' key")
		}
		if name, ok := v["name"].(string); !ok || name != "v3.0" {
			t.Errorf("expected name='v3.0', got %v", v["name"])
		}
		if desc, ok := v["description"].(string); !ok || desc != "Third release" {
			t.Errorf("expected description='Third release', got %v", v["description"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": {
				"id": 3,
				"project": {"id":1,"name":"Test Project"},
				"name": "v3.0",
				"description": "Third release",
				"status": "open",
				"due_date": "2026-01-01",
				"sharing": "none",
				"created_on": "2025-06-01T00:00:00Z",
				"updated_on": "2025-06-01T00:00:00Z"
			}
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	version, err := client.CreateVersion(CreateVersionParams{
		ProjectID:   1,
		Name:        "v3.0",
		Description: "Third release",
		DueDate:     "2026-01-01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version.ID != 3 {
		t.Errorf("expected version ID=3, got %d", version.ID)
	}
	if version.Name != "v3.0" {
		t.Errorf("expected version name='v3.0', got %q", version.Name)
	}
	if version.Status != "open" {
		t.Errorf("expected version status='open', got %q", version.Status)
	}
}

func TestCreateVersion_MissingName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects/1/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":["Name cannot be blank"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	_, err := client.CreateVersion(CreateVersionParams{ProjectID: 1, Name: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected error to contain '422', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateVersion
// ---------------------------------------------------------------------------

func TestUpdateVersion_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /versions/1.json", func(w http.ResponseWriter, r *http.Request) {
		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		v, ok := req["version"]
		if !ok {
			t.Fatal("request body missing 'version' key")
		}
		if status, ok := v["status"].(string); !ok || status != "closed" {
			t.Errorf("expected status='closed', got %v", v["status"])
		}

		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateVersion(UpdateVersionParams{VersionID: 1, Status: "closed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateVersion_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /versions/999.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Not found"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateVersion(UpdateVersionParams{VersionID: 999, Name: "updated"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListWikiPages
// ---------------------------------------------------------------------------

func TestListWikiPages_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/wiki/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"wiki_pages": [
				{
					"title": "Wiki",
					"version": 3,
					"created_on": "2025-01-01T00:00:00Z",
					"updated_on": "2025-03-15T10:30:00Z"
				},
				{
					"title": "Getting_Started",
					"version": 1,
					"created_on": "2025-02-01T00:00:00Z",
					"updated_on": "2025-02-01T00:00:00Z"
				}
			]
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	pages, err := client.ListWikiPages(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 wiki pages, got %d", len(pages))
	}
	if pages[0].Title != "Wiki" {
		t.Errorf("expected first page title='Wiki', got %q", pages[0].Title)
	}
	if pages[0].Version != 3 {
		t.Errorf("expected first page version=3, got %d", pages[0].Version)
	}
	if pages[1].Title != "Getting_Started" {
		t.Errorf("expected second page title='Getting_Started', got %q", pages[1].Title)
	}
}

func TestListWikiPages_Empty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/wiki/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"wiki_pages":[]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	pages, err := client.ListWikiPages(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 wiki pages, got %d", len(pages))
	}
}

// ---------------------------------------------------------------------------
// GetWikiPage
// ---------------------------------------------------------------------------

func TestGetWikiPage_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/wiki/TestPage.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"wiki_page": {
				"title": "TestPage",
				"text": "h1. Test Page\n\nThis is the content of the test page.",
				"version": 5,
				"author": {"id":1,"name":"John Doe"},
				"comments": "Updated formatting",
				"created_on": "2025-01-01T00:00:00Z",
				"updated_on": "2025-04-10T14:30:00Z"
			}
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	page, err := client.GetWikiPage(1, "TestPage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Title != "TestPage" {
		t.Errorf("expected title='TestPage', got %q", page.Title)
	}
	if page.Version != 5 {
		t.Errorf("expected version=5, got %d", page.Version)
	}
	if page.Author.ID != 1 {
		t.Errorf("expected author ID=1, got %d", page.Author.ID)
	}
	if !strings.Contains(page.Text, "Test Page") {
		t.Errorf("expected text to contain 'Test Page', got %q", page.Text)
	}
	if page.Comments != "Updated formatting" {
		t.Errorf("expected comments='Updated formatting', got %q", page.Comments)
	}
}

func TestGetWikiPage_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/wiki/NonExistent.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Not found"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	_, err := client.GetWikiPage(1, "NonExistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateOrUpdateWikiPage
// ---------------------------------------------------------------------------

func TestCreateOrUpdateWikiPage_Create(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /projects/1/wiki/NewPage.json", func(w http.ResponseWriter, r *http.Request) {
		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		wp, ok := req["wiki_page"]
		if !ok {
			t.Fatal("request body missing 'wiki_page' key")
		}
		if text, ok := wp["text"].(string); !ok || text != "h1. New Page\n\nContent here." {
			t.Errorf("expected text='h1. New Page\\n\\nContent here.', got %v", wp["text"])
		}
		if comments, ok := wp["comments"].(string); !ok || comments != "Initial creation" {
			t.Errorf("expected comments='Initial creation', got %v", wp["comments"])
		}

		w.WriteHeader(http.StatusCreated)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.CreateOrUpdateWikiPage(WikiPageParams{
		ProjectID: 1,
		Title:     "NewPage",
		Text:      "h1. New Page\n\nContent here.",
		Comments:  "Initial creation",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateOrUpdateWikiPage_Update(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /projects/1/wiki/ExistingPage.json", func(w http.ResponseWriter, r *http.Request) {
		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse request body: %v", err)
		}
		wp, ok := req["wiki_page"]
		if !ok {
			t.Fatal("request body missing 'wiki_page' key")
		}
		if text, ok := wp["text"].(string); !ok || text != "Updated content" {
			t.Errorf("expected text='Updated content', got %v", wp["text"])
		}

		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.CreateOrUpdateWikiPage(WikiPageParams{
		ProjectID: 1,
		Title:     "ExistingPage",
		Text:      "Updated content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Additional tests for broader coverage
// ---------------------------------------------------------------------------

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	client := NewClient("http://example.com/", "key")
	if client.baseURL != "http://example.com" {
		t.Errorf("expected baseURL without trailing slash, got %q", client.baseURL)
	}
}

func TestDoRequest_SetsHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Redmine-API-Key") != "my-api-key" {
			t.Errorf("expected API key 'my-api-key', got %q", r.Header.Get("X-Redmine-API-Key"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "my-api-key")

	data, err := client.doRequest("GET", "/test.json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Errorf("expected response to contain 'ok', got %q", string(data))
	}
}

func TestUpdateTimeEntry_VerifiesAllFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /time_entries/5.json", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse body: %v", err)
		}
		te := req["time_entry"]
		if te["hours"].(float64) != 3.5 {
			t.Errorf("expected hours=3.5, got %v", te["hours"])
		}
		if te["activity_id"].(float64) != 9 {
			t.Errorf("expected activity_id=9, got %v", te["activity_id"])
		}
		if te["comments"].(string) != "Code review" {
			t.Errorf("expected comments='Code review', got %v", te["comments"])
		}
		if te["spent_on"].(string) != "2025-06-15" {
			t.Errorf("expected spent_on='2025-06-15', got %v", te["spent_on"])
		}
		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateTimeEntry(UpdateTimeEntryParams{
		TimeEntryID: 5,
		Hours:       3.5,
		ActivityID:  9,
		Comments:    "Code review",
		SpentOn:     "2025-06-15",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchUsers_AdminWithNameFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users.json", func(w http.ResponseWriter, r *http.Request) {
		// Verify the name query parameter is passed
		name := r.URL.Query().Get("name")
		if name != "john" {
			t.Errorf("expected name query param 'john', got %q", name)
		}
		status := r.URL.Query().Get("status")
		if status != "1" {
			t.Errorf("expected status query param '1' (default active), got %q", status)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"users": [
				{"id":1,"login":"john","firstname":"John","lastname":"Doe","mail":"john@example.com"}
			],
			"total_count": 1
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	users, total, err := client.SearchUsers(SearchUsersParams{Name: "john"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Name != "John Doe" {
		t.Errorf("expected name='John Doe', got %q", users[0].Name)
	}
}

func TestCreateVersion_WithAllParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects/5/versions.json", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse body: %v", err)
		}
		v := req["version"]
		if v["name"].(string) != "Sprint 42" {
			t.Errorf("expected name='Sprint 42', got %v", v["name"])
		}
		if v["description"].(string) != "Sprint goal" {
			t.Errorf("expected description='Sprint goal', got %v", v["description"])
		}
		if v["status"].(string) != "open" {
			t.Errorf("expected status='open', got %v", v["status"])
		}
		if v["due_date"].(string) != "2026-03-01" {
			t.Errorf("expected due_date='2026-03-01', got %v", v["due_date"])
		}
		if v["sharing"].(string) != "descendants" {
			t.Errorf("expected sharing='descendants', got %v", v["sharing"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": {
				"id": 42,
				"project": {"id":5,"name":"My Project"},
				"name": "Sprint 42",
				"description": "Sprint goal",
				"status": "open",
				"due_date": "2026-03-01",
				"sharing": "descendants",
				"created_on": "2026-02-01T00:00:00Z",
				"updated_on": "2026-02-01T00:00:00Z"
			}
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	version, err := client.CreateVersion(CreateVersionParams{
		ProjectID:   5,
		Name:        "Sprint 42",
		Description: "Sprint goal",
		Status:      "open",
		DueDate:     "2026-03-01",
		Sharing:     "descendants",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version.ID != 42 {
		t.Errorf("expected version ID=42, got %d", version.ID)
	}
	if version.Sharing != "descendants" {
		t.Errorf("expected sharing='descendants', got %q", version.Sharing)
	}
}

func TestUpdateVersion_WithAllFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /versions/10.json", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse body: %v", err)
		}
		v := req["version"]
		if v["name"].(string) != "New Name" {
			t.Errorf("expected name='New Name', got %v", v["name"])
		}
		if v["description"].(string) != "New desc" {
			t.Errorf("expected description='New desc', got %v", v["description"])
		}
		if v["status"].(string) != "locked" {
			t.Errorf("expected status='locked', got %v", v["status"])
		}
		if v["due_date"].(string) != "2026-06-01" {
			t.Errorf("expected due_date='2026-06-01', got %v", v["due_date"])
		}
		if v["sharing"].(string) != "tree" {
			t.Errorf("expected sharing='tree', got %v", v["sharing"])
		}
		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.UpdateVersion(UpdateVersionParams{
		VersionID:   10,
		Name:        "New Name",
		Description: "New desc",
		Status:      "locked",
		DueDate:     "2026-06-01",
		Sharing:     "tree",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateOrUpdateWikiPage_NoComments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /projects/1/wiki/Simple.json", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		var req map[string]map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to parse body: %v", err)
		}
		wp := req["wiki_page"]
		// Comments should not be present when empty
		if _, exists := wp["comments"]; exists {
			t.Error("expected no 'comments' key when comments is empty")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.CreateOrUpdateWikiPage(WikiPageParams{
		ProjectID: 1,
		Title:     "Simple",
		Text:      "Just text, no comments.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteRelation_VerifiesMethod(t *testing.T) {
	mux := http.NewServeMux()
	methodCalled := false
	mux.HandleFunc("DELETE /relations/42.json", func(w http.ResponseWriter, r *http.Request) {
		methodCalled = true
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.DeleteRelation(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !methodCalled {
		t.Error("expected DELETE handler to be called")
	}
}

func TestRemoveWatcher_VerifiesPath(t *testing.T) {
	mux := http.NewServeMux()
	pathCalled := ""
	mux.HandleFunc("DELETE /issues/100/watchers/200.json", func(w http.ResponseWriter, r *http.Request) {
		pathCalled = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	err := client.RemoveWatcher(100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pathCalled != "/issues/100/watchers/200.json" {
		t.Errorf("expected path '/issues/100/watchers/200.json', got %q", pathCalled)
	}
}

func TestDoRequest_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	_, err := client.doRequest("GET", "/test.json", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}

func TestGetWikiPage_SpecialCharactersInTitle(t *testing.T) {
	mux := http.NewServeMux()
	// The title "My Page" gets path-escaped to "My%20Page"
	mux.HandleFunc("GET /projects/1/wiki/My%20Page.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"wiki_page": {
				"title": "My Page",
				"text": "Content with spaces in title",
				"version": 1,
				"author": {"id":1,"name":"Admin"},
				"comments": "",
				"created_on": "2025-01-01T00:00:00Z",
				"updated_on": "2025-01-01T00:00:00Z"
			}
		}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	page, err := client.GetWikiPage(1, "My Page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Title != "My Page" {
		t.Errorf("expected title='My Page', got %q", page.Title)
	}
}

func TestListVersions_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("something went wrong"))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	_, err := client.ListVersions(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}

func TestListWikiPages_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/1/wiki/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := NewClient(ts.URL, "test-key")

	_, err := client.ListWikiPages(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to contain '403', got: %v", err)
	}
}
