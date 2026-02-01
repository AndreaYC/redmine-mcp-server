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

## Usage Examples

Below are real-world examples of using the Redmine MCP tools with Claude Code.

### Check Current User (`me`)

```
User: 測試 Redmine 連線

Claude: [Calls mcp__redmine__whoami]

Result:
{
  "id": 123,
  "login": "john.doe",
  "firstname": "John",
  "lastname": "Doe",
  "mail": "john.doe@example.com",
  "last_login_on": "2026-01-30T03:36:21Z"
}
```

### List Projects

```
User: 列出我可以存取的專案

Claude: [Calls mcp__redmine__list-projects with limit=5]

Result:
| ID  | Identifier        | Name                    |
|-----|-------------------|-------------------------|
| 78  | common-info       | Common Information      |
| 156 | web-platform      | Web Platform            |
| 201 | server-monitoring | Server Monitoring       |
| 245 | devops-tools      | DevOps Tools            |
| 302 | mobile-app        | Mobile Application      |

Total: 50 projects
```

### List My Issues

```
User: 列出指派給我的 open issues

Claude: [Calls mcp__redmine__list-my-issues with statusId="open"]

Result:
| ID     | Project           | Type    | Status      | Subject                          |
|--------|-------------------|---------|-------------|----------------------------------|
| #12345 | Server Monitoring | Bug     | In Progress | API response timeout issue       |
| #12346 | Web Platform      | Feature | New         | Add dark mode support            |
| #12347 | DevOps Tools      | Task    | In Progress | Update deployment scripts        |

Total: 15 open issues
```

### Get Issue Details

```
User: 查看 #12345

Claude: [Calls mcp__redmine__get-issue with issueId="12345"]

Result:
## Issue #12345

| Field       | Value                              |
|-------------|------------------------------------|
| Project     | Server Monitoring                  |
| Type        | Bug                                |
| Status      | In Progress                        |
| Priority    | Normal                             |
| Author      | jane.smith                         |
| Assigned to | john.doe                           |
| Done ratio  | 30%                                |
| Created     | 2026-01-15                         |
| Updated     | 2026-01-28                         |

### Subject
API response timeout issue when fetching large datasets

### Description
When requesting more than 1000 records, the API times out after 30 seconds.

### Journals
**2026-01-20 - jane.smith:**
> Increased timeout to 60 seconds, but issue persists with 5000+ records.
```

### Log Time Entry

```
User: 幫我在 #12345 更新工時 2 小時

Claude: [Calls mcp__redmine__update-issue with issueId="12345", logHours=2]

Result:
Time entry created:
| Field      | Value                              |
|------------|------------------------------------|
| Entry ID   | 56789                              |
| Issue      | #12345 - API response timeout      |
| Hours      | 2                                  |
| Activity   | Development                        |
| Date       | 2026-02-01                         |
| User       | john.doe                           |
```

### Update Issue with Notes

```
User: 在 #12345 加個備註說已經找到問題原因

Claude: [Calls mcp__redmine__update-issue with issueId="12345",
         notes="Found root cause: N+1 query in the pagination logic"]

Result:
Issue #12345 updated successfully.
Note added to journal.
```

### Search Issues by Project

```
User: 找 Server Monitoring 專案的所有 Bug

Claude: [Calls mcp__redmine__list-my-issues with projectId="server-monitoring",
         statusId="open"]

Result:
Found 8 bugs in Server Monitoring:

| ID     | Status      | Subject                            | Updated    |
|--------|-------------|------------------------------------|------------|
| #12345 | In Progress | API response timeout issue         | 2026-01-28 |
| #12350 | New         | Memory leak in data collector      | 2026-01-25 |
| #12355 | Clarifying  | Dashboard shows incorrect metrics  | 2026-01-20 |
...
```

## License

MIT
