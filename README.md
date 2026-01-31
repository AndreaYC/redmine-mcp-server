# Redmine MCP Server

A Model Context Protocol (MCP) server for Redmine, enabling AI assistants (Claude, ChatGPT, Cursor, etc.) to interact with Redmine.

## Features

- **14 MCP Tools** for managing issues, projects, time entries
- **REST API** for ChatGPT GPT Actions
- **Multiple transport modes**: stdio, SSE, HTTP
- **Smart name resolution**: Use names instead of IDs (projects, trackers, users, etc.)
- **Custom fields support**: Pass custom fields by name

## Quick Start

### Local Usage (Claude Desktop)

1. Build the binary:
```bash
make build
```

2. Configure Claude Desktop (`~/.claude/claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "redmine": {
      "command": "/path/to/server",
      "args": ["mcp"],
      "env": {
        "REDMINE_URL": "http://your-redmine-server",
        "REDMINE_API_KEY": "your-api-key"
      }
    }
  }
}
```

### Docker Deployment (Team Usage)

```bash
docker run -p 8080:8080 \
  -e REDMINE_URL=http://your-redmine-server \
  harbor.sw.ciot.work/mcp/redmine:latest api
```

Each user passes their API key via header:
```
X-Redmine-API-Key: user-api-key
```

## Execution Modes

| Command | Transport | API Key Source | Use Case |
|---------|-----------|----------------|----------|
| `./server mcp` | stdio | `REDMINE_API_KEY` env | Claude Desktop |
| `./server mcp --sse` | SSE/HTTP | Header | Docker, Cursor/Cline |
| `./server api` | REST/HTTP | Header | ChatGPT GPT Actions |

## MCP Tools

### Issues
- `issues.search` - Search issues by project, status, assignee
- `issues.getById` - Get issue details with journals and relations
- `issues.create` - Create new issue with custom fields
- `issues.update` - Update status, assignee, add notes
- `issues.createSubtask` - Create subtask under parent issue
- `issues.addWatcher` - Add watcher to issue
- `issues.addRelation` - Create relation between issues

### Projects
- `projects.list` - List all projects
- `projects.create` - Create new project

### Time Entries
- `timeEntries.create` - Log time on issue

### Reference
- `trackers.list` - List all trackers
- `statuses.list` - List all issue statuses
- `activities.list` - List time entry activities
- `me` - Get current user info

## Name Resolution

All tools support using names instead of IDs:

```json
{
  "project": "SKY Rack Mgmt Software",
  "tracker": "Bug",
  "assigned_to": "Andrea.Ho",
  "status": "In Progress"
}
```

If a name matches multiple entries, the server returns candidates for clarification.

## Custom Fields

Pass custom fields by name:

```json
{
  "custom_fields": {
    "Severity": "Major",
    "HW Version": "L11 SKYRack Cabinet",
    "Issue Finder": "SWQA"
  }
}
```

## REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/me` | Current user |
| GET | `/api/v1/projects` | List projects |
| POST | `/api/v1/projects` | Create project |
| GET | `/api/v1/issues` | Search issues |
| GET | `/api/v1/issues/:id` | Get issue |
| POST | `/api/v1/issues` | Create issue |
| PATCH | `/api/v1/issues/:id` | Update issue |
| POST | `/api/v1/issues/:id/subtasks` | Create subtask |
| POST | `/api/v1/issues/:id/watchers` | Add watcher |
| POST | `/api/v1/issues/:id/relations` | Add relation |
| POST | `/api/v1/time_entries` | Create time entry |
| GET | `/api/v1/trackers` | List trackers |
| GET | `/api/v1/statuses` | List statuses |
| GET | `/api/v1/activities` | List activities |

API documentation available at `/docs` (Swagger UI) and `/openapi.yaml`.

## Development

```bash
# Install dependencies
go mod download

# Run locally
make run-api     # REST API on :8080
make run-mcp     # MCP stdio mode
make run-sse     # MCP SSE mode on :8080

# Build
make build

# Test
make test

# Lint
make lint

# Build Docker image
make docker-build
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REDMINE_URL` | Redmine server URL | (required) |
| `REDMINE_API_KEY` | API key (stdio mode) | - |
| `PORT` | HTTP server port | 8080 |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | info |

## Client Configuration Examples

### Claude Code (`~/.claude/settings.json`)
```json
{
  "mcpServers": {
    "redmine": {
      "command": "/path/to/server",
      "args": ["mcp"],
      "env": {
        "REDMINE_URL": "http://your-redmine-server",
        "REDMINE_API_KEY": "your-api-key"
      }
    }
  }
}
```

### Cursor/Cline (SSE mode)
```json
{
  "mcpServers": {
    "redmine": {
      "url": "http://localhost:8080/sse",
      "headers": {
        "X-Redmine-API-Key": "your-api-key"
      }
    }
  }
}
```

### ChatGPT GPT Actions

1. Go to GPT editor
2. Add new Action
3. Import from `http://your-server/openapi.yaml`
4. Set Authentication: API Key in header `X-Redmine-API-Key`

## License

MIT
