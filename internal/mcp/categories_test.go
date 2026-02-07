package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

func TestCategoriesHandlers(t *testing.T) {
	client := &redmine.Client{}
	handlers := NewToolHandlers(client, nil, nil)
	ctx := context.Background()

	t.Run("handleCategoriesCreate requires read-only check", func(t *testing.T) {
		// Enable read-only mode
		handlers.readOnly = true
		defer func() { handlers.readOnly = false }()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"project": "test",
			"name":    "Bug Category",
		}

		result, err := handlers.handleCategoriesCreate(ctx, req)
		if err != nil {
			t.Fatalf("handler should not return error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result in read-only mode")
		}
	})

	t.Run("handleCategoriesUpdate requires read-only check", func(t *testing.T) {
		handlers.readOnly = true
		defer func() { handlers.readOnly = false }()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"category_id": float64(1),
			"name":        "Updated Category",
		}

		result, err := handlers.handleCategoriesUpdate(ctx, req)
		if err != nil {
			t.Fatalf("handler should not return error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result in read-only mode")
		}
	})

	t.Run("handleCategoriesDelete requires read-only check", func(t *testing.T) {
		handlers.readOnly = true
		defer func() { handlers.readOnly = false }()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"category_id": float64(1),
		}

		result, err := handlers.handleCategoriesDelete(ctx, req)
		if err != nil {
			t.Fatalf("handler should not return error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result in read-only mode")
		}
	})

}
