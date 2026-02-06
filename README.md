# Redmine MCP Server

A Model Context Protocol (MCP) server for Redmine, enabling AI assistants (Claude, ChatGPT, Cursor, etc.) to interact with Redmine.

## Features

- **27 MCP Tools** for managing issues, projects, time entries, attachments, and more
- **REST API** for ChatGPT GPT Actions and scripting
- **Multiple transport modes**: stdio, SSE, HTTP
- **Smart name resolution**: Use names instead of IDs (projects, trackers, users, etc.)
- **Custom fields support**: Pass custom fields by name with auto-correction
- **Workflow validation**: Status transition validation prevents invalid updates
- **File attachments**: Upload/download files via base64 (MCP) or multipart form (REST)

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
| `./server mcp --sse` | SSE + Streamable HTTP | Header | Docker, Cursor/Cline, Codex |
| `./server api` | REST/HTTP | Header | ChatGPT GPT Actions |

The `--sse` mode serves both SSE (`/sse`, `/message`) and Streamable HTTP (`/mcp`) transports on the same port.

### Authentication

Both authentication methods are supported:
- `X-Redmine-API-Key: <api-key>` header (Cursor, Cline, Claude Code)
- `Authorization: Bearer <api-key>` header (Codex)

## MCP Tools

### Account
- `me` - Get current user info

### Projects
- `projects_list` - List all projects
- `projects_create` - Create new project
- `projects_getDetail` - Get project details (trackers, custom fields)
- `projects_update` - Update project settings (requires admin/manager)

### Issues
- `issues_search` - Search issues by project, status, assignee, dates, custom fields
- `issues_getById` - Get issue details with journals, relations, and attachments
- `issues_create` - Create new issue with custom fields and attachments
- `issues_update` - Update status, assignee, add notes, attach files
- `issues_createSubtask` - Create subtask under parent issue
- `issues_addWatcher` - Add watcher to issue
- `issues_removeWatcher` - Remove watcher from issue
- `issues_addRelation` - Create relation between issues
- `issues_removeRelation` - Remove relation between issues
- `issues_getRequiredFields` - Get required fields for creating issues
- `issues_batchUpdate` - Batch update multiple issues
- `issues_copy` - Copy an issue to another project
- `issues_exportCSV` - Export issues to CSV format

### Custom Fields
- `customFields_list` - List custom fields for a project/tracker

### Time Entries
- `timeEntries_create` - Log time on issue
- `timeEntries_list` - List time entries with filters
- `timeEntries_report` - Generate aggregated time reports
- `timeEntries_update` - Update a time entry
- `timeEntries_delete` - Delete a time entry

### Attachments
- `attachments_upload` - Upload a file, get upload token
- `attachments_download` - Download attachment (returns base64)
- `attachments_list` - List attachments on an issue
- `attachments_uploadAndAttach` - Upload and attach to issue in one step

### Versions
- `versions_list` - List versions in a project
- `versions_create` - Create a new version
- `versions_update` - Update a version

### Wiki
- `wiki_list` - List wiki pages in a project
- `wiki_get` - Get wiki page content
- `wiki_createOrUpdate` - Create or update a wiki page

### Users
- `users_search` - Search users by name

### Reports
- `reports_weekly` - Generate weekly report
- `reports_standup` - Generate standup report
- `reports_project_analysis` - Comprehensive project analysis (time tracking, issues, custom fields, trends)
- `reports_projects_compare` - Compare multiple projects side by side

### Reference
- `trackers_list` - List all trackers
- `statuses_list` - List all issue statuses
- `priorities_list` - List all issue priorities
- `activities_list` - List time entry activities
- `reference_workflow` - Show workflow transition rules

## Project Analysis Reports

### reports_project_analysis

Generate comprehensive project analysis for resource planning and project retrospectives.

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `project` | ✓ | Project name or ID |
| `from` | | Start date (YYYY-MM-DD) |
| `to` | | End date (YYYY-MM-DD) |
| `issue_status` | | Filter: `all` (default), `open`, `closed` |
| `version` | | Filter by version/milestone |
| `custom_fields` | | Custom fields to analyze (comma-separated, default: all) |
| `format` | | Output: `json` (default), `csv`, `excel` |
| `attach_to` | | Save location: `dmsf`, `dmsf:FolderID`, `files`, `wiki`, `wiki:PageName`, `issue:123` |

**Output includes:**
- Summary: total hours, person days, contributors, issue counts
- By Tracker: hours and issue count per tracker type
- By User: hours per contributor
- By Version: hours per milestone
- By Custom Field: breakdown by Component, SW_Category, etc.
- Monthly Trend: hours over time
- Top Issues: most time-consuming issues

### reports_projects_compare

Compare multiple projects side by side for benchmarking.

**Parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `projects` | ✓ | Project names/IDs (comma-separated) |
| `from` | | Start date |
| `to` | | End date |
| `issue_status` | | Filter: `all`, `open`, `closed` |
| `format` | | Output: `json`, `csv`, `excel` |
| `attach_to` | | Save location: `dmsf`, `dmsf:FolderID`, `files`, `wiki`, `issue:123` |
| `target_project` | | Project to save report (required with attach_to) |

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

### Custom Field Validation

When configured with `CUSTOM_FIELD_RULES_FILE`, the server validates custom field values and auto-corrects case mismatches. For example, `"sw tool"` is auto-corrected to `"SW Tool"`.

### Workflow Validation

When configured with `WORKFLOW_RULES_FILE`, the server validates status transitions before sending to Redmine, preventing silent failures from invalid transitions.

## Attachments

### MCP (AI Assistants)

Files are transferred as base64-encoded strings (max 3MB decoded).

**One-step upload and attach (most common):**
```json
{
  "tool": "attachments.uploadAndAttach",
  "arguments": {
    "issue_id": 12345,
    "filename": "report.txt",
    "content": "SGVsbG8gV29ybGQ=",
    "description": "Test report"
  }
}
```

**Two-step flow (upload then attach during create/update):**
```json
// Step 1: Upload
{"tool": "attachments.upload", "arguments": {"filename": "report.txt", "content": "SGVsbG8gV29ybGQ="}}
// Returns: {"token": "abc123", "filename": "report.txt"}

// Step 2: Attach via issues.update
{"tool": "issues.update", "arguments": {"issue_id": 12345, "upload_tokens": [{"token": "abc123", "filename": "report.txt"}]}}
```

### REST API (Scripts/Tools)

Files are uploaded via multipart form (max 5MB per file).

**One-step attach:**
```bash
curl -X POST http://localhost:8080/api/v1/issues/12345/attach \
  -H "X-Redmine-API-Key: your-key" \
  -F "files[]=@report.pdf" \
  -F "files[]=@screenshot.png" \
  -F "notes=Attached reports"
```

**Two-step flow:**
```bash
# Step 1: Upload
curl -X POST http://localhost:8080/api/v1/attachments/upload \
  -H "X-Redmine-API-Key: your-key" \
  -F "file=@report.pdf"
# Returns: {"token": "abc123", "filename": "report.pdf"}

# Step 2: Attach via issue update
curl -X PATCH http://localhost:8080/api/v1/issues/12345 \
  -H "X-Redmine-API-Key: your-key" \
  -H "Content-Type: application/json" \
  -d '{"upload_tokens": [{"token": "abc123", "filename": "report.pdf"}]}'
```

**Download:**
```bash
curl -o report.pdf http://localhost:8080/api/v1/attachments/42/download \
  -H "X-Redmine-API-Key: your-key"
```

## REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/me` | Current user |
| GET | `/api/v1/projects` | List projects |
| POST | `/api/v1/projects` | Create project |
| GET | `/api/v1/projects/:id` | Get project details |
| PATCH | `/api/v1/projects/:id` | Update project |
| GET | `/api/v1/issues` | Search issues |
| GET | `/api/v1/issues/:id` | Get issue |
| POST | `/api/v1/issues` | Create issue |
| PATCH | `/api/v1/issues/:id` | Update issue |
| POST | `/api/v1/issues/:id/subtasks` | Create subtask |
| POST | `/api/v1/issues/:id/watchers` | Add watcher |
| POST | `/api/v1/issues/:id/relations` | Add relation |
| GET | `/api/v1/issues/:id/attachments` | List issue attachments |
| POST | `/api/v1/issues/:id/attach` | Attach files to issue |
| POST | `/api/v1/attachments/upload` | Upload file |
| GET | `/api/v1/attachments/:id/download` | Download attachment |
| POST | `/api/v1/time_entries` | Create time entry |
| GET | `/api/v1/custom_fields` | List custom fields (admin) |
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
| `CUSTOM_FIELD_RULES_FILE` | Path to custom field validation rules JSON | - |
| `WORKFLOW_RULES_FILE` | Path to workflow transition rules JSON | - |

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

### OpenAI Codex CLI (`~/.codex/config.toml`)
```toml
[mcp_servers.redmine]
url = "http://localhost:8080/mcp"
bearer_token_env_var = "REDMINE_API_KEY"
```

Set the environment variable:
```bash
export REDMINE_API_KEY="your-api-key"
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

### Upload and Attach File

```
User: 把這個測試報告附加到 #12345

Claude: [Calls attachments.uploadAndAttach with issue_id=12345,
         filename="test_report.txt", content="<base64>"]

Result:
File uploaded and attached to issue #12345 successfully.
- Filename: test_report.txt
- Size: 1,234 bytes
```

### Log Time Entry

```
User: Log 2 hours on #12345

Claude: [Calls mcp__redmine__timeEntries_create with issue_id=12345, hours=2]

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

### Log Time Entry for a Specific Date

```
User: Log 8 hours PTO for last Monday (2026-01-27) on #15481

Claude: [Calls mcp__redmine__timeEntries_create with issue_id=15481, hours=8,
         spent_on="2026-01-27", comments="PTO"]

Result:
Time entry created:
| Field      | Value                              |
|------------|------------------------------------|
| Entry ID   | 56790                              |
| Issue      | #15481 - PTO/Sick leave/Holiday    |
| Hours      | 8                                  |
| Activity   | Others                             |
| Date       | 2026-01-27                         |
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

## Time Entry Analysis

The `timeEntries.list` and `timeEntries.report` tools enable powerful time tracking analysis.

### Available Parameters

| Parameter | Description | Example |
|-----------|-------------|---------|
| `project` | Filter by project (name or ID) | `"1306"` |
| `user` | Filter by user, use `"me"` for current user | `"me"`, `"john.doe"` |
| `from` / `to` | Date range (YYYY-MM-DD) | `"2026-01-01"` |
| `period` | Date shortcut | `"this_week"`, `"last_week"`, `"this_month"`, `"last_month"` |
| `group_by` | Aggregation dimensions (report only) | `"project"`, `"user"`, `"activity"`, `"project,user"` |

### Analysis Examples

#### 1. My Time This Week

```
User: How much time did I log this week?

Claude: [Calls timeEntries.list with user="me", period="this_week"]

Result:
{
  "total_count": 5,
  "time_entries": [
    {"project": "SKY Rack", "hours": 3, "activity": "Development", "spent_on": "2026-02-03"},
    {"project": "SKY Rack", "hours": 2.5, "activity": "Code Review", "spent_on": "2026-02-02"},
    ...
  ]
}
```

#### 2. Team Hours by Person (Last Month)

```
User: Show me last month's hours by team member

Claude: [Calls timeEntries.report with period="last_month", group_by="user"]

Result:
| Rank | User        | Hours |
|------|-------------|-------|
| 1    | Eric.Hsu    | 184h  |
| 2    | Willy.Yao   | 179h  |
| 3    | Deron.Chen  | 177h  |
| ...  | (36 people) | ...   |

Total: 3,321 hours (923 entries)
```

#### 3. Project Hours by Activity Type

```
User: How is time distributed across activities in project 1306?

Claude: [Calls timeEntries.report with project="1306", period="last_month", group_by="activity"]

Result:
| Activity    | Hours   | Percentage |
|-------------|---------|------------|
| Development | 178.25h | 63%        |
| Others      | 96.9h   | 34%        |
| Study Spec  | 3.5h    | 1%         |
| Debug       | 3.5h    | 1%         |

Total: 282 hours
```

#### 4. Multi-Dimensional Analysis (Project + Activity)

```
User: Show hours breakdown by project and activity

Claude: [Calls timeEntries.report with period="last_month", group_by="project,activity"]

Result:
| Project                  | Activity    | Hours |
|--------------------------|-------------|-------|
| Platform Management Team | Others      | 423h  |
| OS/BSP team              | Others      | 325h  |
| SKY Rack Mgmt Software   | Development | 178h  |
| NCG_BIOS_Task            | Development | 167h  |
| ...                      | ...         | ...   |
```

### Common Analysis Scenarios

| Scenario | Query Parameters |
|----------|------------------|
| My hours this week | `user="me", period="this_week"` |
| Who spent most time on a project? | `project="X", period="last_month", group_by="user"` |
| Team activity distribution | `period="last_month", group_by="activity"` |
| Which projects consume most time? | `period="last_month", group_by="project"` |
| One person's project allocation | `user="X", period="last_month", group_by="project"` |
| Detailed breakdown per project | `project="X", group_by="user,activity"` |

## License

MIT
