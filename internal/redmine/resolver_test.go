package redmine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolver_ResolveProject(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/projects.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"projects": [
					{"id": 1, "name": "Test Project", "identifier": "test-project"},
					{"id": 2, "name": "Another Project", "identifier": "another"}
				]
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resolver := NewResolver(client)

	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"by ID", "1", 1, false},
		{"by name exact", "Test Project", 1, false},
		{"by name case insensitive", "test project", 1, false},
		{"by identifier", "test-project", 1, false},
		{"not found", "nonexistent", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.ResolveProject(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveProject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveProject() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolver_ResolveTracker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/trackers.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"trackers": [
					{"id": 1, "name": "Bug"},
					{"id": 2, "name": "Feature"},
					{"id": 3, "name": "Task"}
				]
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resolver := NewResolver(client)

	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"by ID", "2", 2, false},
		{"by name exact", "Bug", 1, false},
		{"by name case insensitive", "feature", 2, false},
		{"not found", "nonexistent", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.ResolveTracker(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveTracker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveTracker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolver_ResolveStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/issue_statuses.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issue_statuses": [
					{"id": 1, "name": "New", "is_closed": false},
					{"id": 2, "name": "In Progress", "is_closed": false},
					{"id": 3, "name": "Closed", "is_closed": true}
				]
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resolver := NewResolver(client)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"special: open", "open", "open", false},
		{"special: closed", "closed", "closed", false},
		{"special: all", "all", "*", false},
		{"by ID", "1", "1", false},
		{"by name exact", "New", "1", false},
		{"by name case insensitive", "in progress", "2", false},
		{"not found", "nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.ResolveStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ResolveStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  ResolveError
		want string
	}{
		{
			name: "not found",
			err:  ResolveError{Type: "project", Query: "foo", NotFound: true},
			want: "project not found: foo",
		},
		{
			name: "multiple matches",
			err: ResolveError{
				Type:    "tracker",
				Query:   "task",
				Matches: []IDName{{ID: 1, Name: "Task"}, {ID: 2, Name: "Subtask"}},
			},
			want: "multiple tracker match 'task': Task (ID: 1), Subtask (ID: 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("ResolveError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
