package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

func TestMembershipsHandlers(t *testing.T) {
	client := &redmine.Client{}
	handlers := NewToolHandlers(client, nil, nil)
	ctx := context.Background()

	t.Run("handleMembershipsAdd requires read-only check", func(t *testing.T) {
		// Enable read-only mode
		handlers.readOnly = true
		defer func() { handlers.readOnly = false }()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"project": "test",
			"user":    "john",
			"roles":   []interface{}{"Manager"},
		}

		result, err := handlers.handleMembershipsAdd(ctx, req)
		if err != nil {
			t.Fatalf("handler should not return error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result in read-only mode")
		}
	})

	t.Run("handleMembershipsUpdate requires read-only check", func(t *testing.T) {
		handlers.readOnly = true
		defer func() { handlers.readOnly = false }()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"membership_id": float64(1),
			"roles":         []interface{}{"Developer"},
		}

		result, err := handlers.handleMembershipsUpdate(ctx, req)
		if err != nil {
			t.Fatalf("handler should not return error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result in read-only mode")
		}
	})

	t.Run("handleMembershipsRemove requires read-only check", func(t *testing.T) {
		handlers.readOnly = true
		defer func() { handlers.readOnly = false }()

		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"membership_id": float64(1),
		}

		result, err := handlers.handleMembershipsRemove(ctx, req)
		if err != nil {
			t.Fatalf("handler should not return error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result in read-only mode")
		}
	})

}
