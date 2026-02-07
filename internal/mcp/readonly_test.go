package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

func TestReadOnlyMode(t *testing.T) {
	// Save original env var
	originalValue := os.Getenv("REDMINE_MCP_READ_ONLY")
	defer os.Setenv("REDMINE_MCP_READ_ONLY", originalValue)

	// Enable read-only mode
	os.Setenv("REDMINE_MCP_READ_ONLY", "true")

	// Create handlers with read-only mode enabled
	client := &redmine.Client{}
	handlers := NewToolHandlers(client, nil, nil)

	// Test that checkReadOnly returns an error
	err := handlers.checkReadOnly()
	if err == nil {
		t.Fatal("expected error in read-only mode, got nil")
	}
	if err.Error() != "server is in read-only mode - write operations are disabled" {
		t.Errorf("unexpected error message: %v", err)
	}

	// Test that a write operation returns an error
	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"project": "test",
		"tracker": "Bug",
		"subject": "test issue",
	}

	result, err := handlers.handleIssuesCreate(ctx, req)
	if err != nil {
		t.Fatalf("handler should not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result in read-only mode")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error content")
	}
	errorText := ""
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			errorText = textContent.Text
			break
		}
	}
	if errorText != "server is in read-only mode - write operations are disabled" {
		t.Errorf("unexpected error text: %s", errorText)
	}

	// Disable read-only mode
	os.Setenv("REDMINE_MCP_READ_ONLY", "false")
	handlers = NewToolHandlers(client, nil, nil)

	// Test that checkReadOnly returns nil
	err = handlers.checkReadOnly()
	if err != nil {
		t.Fatalf("expected no error when read-only mode is disabled, got: %v", err)
	}
}
