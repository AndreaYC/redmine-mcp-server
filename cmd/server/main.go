package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ycho/redmine-mcp-server/internal/api"
	"github.com/ycho/redmine-mcp-server/internal/mcp"
)

var (
	version = "1.0.0"

	// Global flags
	redmineURL string
	port       int
	logLevel   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "redmine-mcp-server",
		Short:   "Redmine MCP Server - AI assistant integration for Redmine",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging()
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&redmineURL, "redmine-url", os.Getenv("REDMINE_URL"), "Redmine server URL")
	rootCmd.PersistentFlags().IntVar(&port, "port", 8080, "Server port (for SSE and API modes)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	// MCP command
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server",
		Long:  "Start the MCP server in stdio or SSE mode",
		RunE:  runMCP,
	}

	var sseMode bool
	mcpCmd.Flags().BoolVar(&sseMode, "sse", false, "Run in SSE mode instead of stdio")

	// API command
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "Start REST API server",
		Long:  "Start the REST API server for ChatGPT GPT Actions",
		RunE:  runAPI,
	}

	rootCmd.AddCommand(mcpCmd, apiCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupLogging() {
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

func runMCP(cmd *cobra.Command, args []string) error {
	if redmineURL == "" {
		return fmt.Errorf("REDMINE_URL is required (set via --redmine-url or REDMINE_URL env var)")
	}

	sseMode, _ := cmd.Flags().GetBool("sse")

	config := mcp.Config{
		RedmineURL:    redmineURL,
		RedmineAPIKey: os.Getenv("REDMINE_API_KEY"),
		Port:          port,
		SSEMode:       sseMode,
	}

	if !sseMode && config.RedmineAPIKey == "" {
		return fmt.Errorf("REDMINE_API_KEY is required for stdio mode (set via REDMINE_API_KEY env var)")
	}

	server := mcp.NewServer(config)
	return server.Run()
}

func runAPI(cmd *cobra.Command, args []string) error {
	if redmineURL == "" {
		return fmt.Errorf("REDMINE_URL is required (set via --redmine-url or REDMINE_URL env var)")
	}

	config := api.Config{
		RedmineURL: redmineURL,
		Port:       port,
	}

	server := api.NewServer(config)
	return server.Run()
}
