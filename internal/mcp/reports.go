package mcp

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ycho/redmine-mcp-server/internal/redmine"
)

// ProjectAnalysisParams are parameters for project analysis
type ProjectAnalysisParams struct {
	ProjectID    int
	ProjectName  string
	From         string
	To           string
	IssueStatus  string   // "all", "open", "closed"
	Version      string   // filter by version
	CustomFields []string // specific custom fields to analyze (empty = all)
	Format       string   // "json", "csv"
	AttachTo     string   // "dmsf", "dmsf:FolderID", "files", "files:VersionName", "issue:ID"
}

// ProjectAnalysisResult is the result of project analysis
type ProjectAnalysisResult struct {
	Project  map[string]any   `json:"project"`
	Period   string           `json:"period,omitempty"`
	Filters  map[string]any   `json:"filters"`
	Summary  map[string]any   `json:"summary"`
	ByTracker     []map[string]any `json:"by_tracker"`
	ByUser        []map[string]any `json:"by_user"`
	ByActivity    []map[string]any `json:"by_activity"`
	ByVersion     []map[string]any `json:"by_version"`
	ByCustomField map[string][]map[string]any `json:"by_custom_field"`
	MonthlyTrend  []map[string]any `json:"monthly_trend"`
	TopIssues     []map[string]any `json:"top_issues"`
	UnlinkedHours float64          `json:"unlinked_hours"`
	DownloadURL   string           `json:"download_url,omitempty"`
}

// ReportGenerator generates project analysis reports
type ReportGenerator struct {
	client   *redmine.Client
	resolver *redmine.Resolver
}

// NewReportGenerator creates a new report generator
func NewReportGenerator(client *redmine.Client, resolver *redmine.Resolver) *ReportGenerator {
	return &ReportGenerator{
		client:   client,
		resolver: resolver,
	}
}

// GenerateProjectAnalysis generates a project analysis report
func (rg *ReportGenerator) GenerateProjectAnalysis(params ProjectAnalysisParams) (*ProjectAnalysisResult, error) {
	result := &ProjectAnalysisResult{
		Project: map[string]any{
			"id":   params.ProjectID,
			"name": params.ProjectName,
		},
		Filters: map[string]any{
			"issue_status": params.IssueStatus,
		},
		ByCustomField: make(map[string][]map[string]any),
	}

	if params.Version != "" {
		result.Filters["version"] = params.Version
	}
	if params.From != "" || params.To != "" {
		result.Period = fmt.Sprintf("%s ~ %s", params.From, params.To)
	}

	// Fetch all time entries for the project
	allEntries, err := rg.fetchAllTimeEntries(params)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch time entries: %w", err)
	}

	// Fetch all issues for the project
	allIssues, err := rg.fetchAllIssues(params)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issues: %w", err)
	}

	// Build issue map for quick lookup
	issueMap := make(map[int]redmine.Issue)
	for _, issue := range allIssues {
		issueMap[issue.ID] = issue
	}

	// Calculate all aggregations
	rg.calculateSummary(result, allEntries, allIssues, issueMap)
	rg.calculateByTracker(result, allEntries, allIssues, issueMap)
	rg.calculateByUser(result, allEntries)
	rg.calculateByActivity(result, allEntries)
	rg.calculateByVersion(result, allEntries, allIssues, issueMap)
	rg.calculateByCustomField(result, allEntries, allIssues, issueMap, params.CustomFields)
	rg.calculateMonthlyTrend(result, allEntries)
	rg.calculateTopIssues(result, allEntries, issueMap)

	return result, nil
}

// fetchAllTimeEntries fetches all time entries for the project with pagination
func (rg *ReportGenerator) fetchAllTimeEntries(params ProjectAnalysisParams) ([]redmine.TimeEntry, error) {
	teParams := redmine.ListTimeEntriesParams{
		ProjectID: strconv.Itoa(params.ProjectID),
		From:      params.From,
		To:        params.To,
		Limit:     100,
	}

	var allEntries []redmine.TimeEntry
	for {
		entries, _, err := rg.client.ListTimeEntries(teParams)
		if err != nil {
			return nil, err
		}
		allEntries = append(allEntries, entries...)
		if len(entries) < teParams.Limit {
			break
		}
		teParams.Offset += teParams.Limit
	}

	return allEntries, nil
}

// fetchAllIssues fetches all issues for the project with pagination
func (rg *ReportGenerator) fetchAllIssues(params ProjectAnalysisParams) ([]redmine.Issue, error) {
	issueParams := redmine.SearchIssuesParams{
		ProjectID: strconv.Itoa(params.ProjectID),
		Limit:     100,
	}

	// Handle issue status filter
	switch params.IssueStatus {
	case "open":
		issueParams.StatusID = "open"
	case "closed":
		issueParams.StatusID = "closed"
	default:
		issueParams.StatusID = "*" // all
	}

	// Handle version filter
	if params.Version != "" {
		versionID, err := rg.resolver.ResolveVersion(params.Version, params.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve version: %w", err)
		}
		issueParams.VersionID = strconv.Itoa(versionID)
	}

	var allIssues []redmine.Issue
	for {
		issues, total, err := rg.client.SearchIssues(issueParams)
		if err != nil {
			return nil, err
		}
		allIssues = append(allIssues, issues...)
		if len(allIssues) >= total {
			break
		}
		issueParams.Offset += issueParams.Limit
	}

	return allIssues, nil
}

// calculateSummary calculates the summary statistics
func (rg *ReportGenerator) calculateSummary(result *ProjectAnalysisResult, entries []redmine.TimeEntry, issues []redmine.Issue, issueMap map[int]redmine.Issue) {
	var totalHours float64
	contributors := make(map[int]bool)

	for _, entry := range entries {
		totalHours += entry.Hours
		contributors[entry.User.ID] = true
	}

	closedCount := 0
	openCount := 0
	for _, issue := range issues {
		if issue.ClosedOn != "" {
			closedCount++
		} else {
			openCount++
		}
	}

	contributorCount := len(contributors)
	var avgHoursPerPerson float64
	if contributorCount > 0 {
		avgHoursPerPerson = totalHours / float64(contributorCount)
	}

	result.Summary = map[string]any{
		"total_hours":          roundFloat(totalHours, 2),
		"total_issues":         len(issues),
		"closed_issues":        closedCount,
		"open_issues":          openCount,
		"contributors":         contributorCount,
		"avg_hours_per_person": roundFloat(avgHoursPerPerson, 2),
		"person_days":          roundFloat(totalHours/8.0, 2),
	}
}

// calculateByTracker aggregates hours by tracker
func (rg *ReportGenerator) calculateByTracker(result *ProjectAnalysisResult, entries []redmine.TimeEntry, issues []redmine.Issue, issueMap map[int]redmine.Issue) {
	trackerHours := make(map[string]float64)
	trackerIssues := make(map[string]map[int]bool)

	for _, entry := range entries {
		if entry.Issue == nil {
			continue
		}
		if issue, ok := issueMap[entry.Issue.ID]; ok {
			tracker := issue.Tracker.Name
			trackerHours[tracker] += entry.Hours
			if trackerIssues[tracker] == nil {
				trackerIssues[tracker] = make(map[int]bool)
			}
			trackerIssues[tracker][entry.Issue.ID] = true
		}
	}

	var byTracker []map[string]any
	for tracker, hours := range trackerHours {
		issueCount := len(trackerIssues[tracker])
		avgHours := 0.0
		if issueCount > 0 {
			avgHours = hours / float64(issueCount)
		}
		byTracker = append(byTracker, map[string]any{
			"tracker":     tracker,
			"hours":       roundFloat(hours, 2),
			"issue_count": issueCount,
			"avg_hours":   roundFloat(avgHours, 2),
		})
	}

	sort.Slice(byTracker, func(i, j int) bool {
		return byTracker[i]["hours"].(float64) > byTracker[j]["hours"].(float64)
	})

	result.ByTracker = byTracker
}

// calculateByUser aggregates hours by user
func (rg *ReportGenerator) calculateByUser(result *ProjectAnalysisResult, entries []redmine.TimeEntry) {
	userHours := make(map[string]float64)
	userIssues := make(map[string]map[int]bool)

	for _, entry := range entries {
		user := entry.User.Name
		userHours[user] += entry.Hours
		if entry.Issue != nil {
			if userIssues[user] == nil {
				userIssues[user] = make(map[int]bool)
			}
			userIssues[user][entry.Issue.ID] = true
		}
	}

	var byUser []map[string]any
	for user, hours := range userHours {
		byUser = append(byUser, map[string]any{
			"user":        user,
			"hours":       roundFloat(hours, 2),
			"issue_count": len(userIssues[user]),
		})
	}

	sort.Slice(byUser, func(i, j int) bool {
		return byUser[i]["hours"].(float64) > byUser[j]["hours"].(float64)
	})

	result.ByUser = byUser
}

// calculateByActivity aggregates hours by activity
func (rg *ReportGenerator) calculateByActivity(result *ProjectAnalysisResult, entries []redmine.TimeEntry) {
	activityHours := make(map[string]float64)
	var totalHours float64

	for _, entry := range entries {
		activityHours[entry.Activity.Name] += entry.Hours
		totalHours += entry.Hours
	}

	var byActivity []map[string]any
	for activity, hours := range activityHours {
		percentage := 0.0
		if totalHours > 0 {
			percentage = (hours / totalHours) * 100
		}
		byActivity = append(byActivity, map[string]any{
			"activity":   activity,
			"hours":      roundFloat(hours, 2),
			"percentage": roundFloat(percentage, 2),
		})
	}

	sort.Slice(byActivity, func(i, j int) bool {
		return byActivity[i]["hours"].(float64) > byActivity[j]["hours"].(float64)
	})

	result.ByActivity = byActivity
}

// calculateByVersion aggregates hours by version
func (rg *ReportGenerator) calculateByVersion(result *ProjectAnalysisResult, entries []redmine.TimeEntry, issues []redmine.Issue, issueMap map[int]redmine.Issue) {
	// Build issue to version mapping
	issueVersionMap := make(map[int]string)
	for _, issue := range issues {
		if issue.FixedVersion != nil {
			issueVersionMap[issue.ID] = issue.FixedVersion.Name
		} else {
			issueVersionMap[issue.ID] = "(none)"
		}
	}

	versionHours := make(map[string]float64)
	versionIssues := make(map[string]map[int]bool)

	for _, entry := range entries {
		var versionName string
		if entry.Issue != nil {
			if v, ok := issueVersionMap[entry.Issue.ID]; ok {
				versionName = v
			} else {
				versionName = "(none)"
			}
		} else {
			versionName = "(none)"
		}

		versionHours[versionName] += entry.Hours
		if entry.Issue != nil {
			if versionIssues[versionName] == nil {
				versionIssues[versionName] = make(map[int]bool)
			}
			versionIssues[versionName][entry.Issue.ID] = true
		}
	}

	var byVersion []map[string]any
	for version, hours := range versionHours {
		byVersion = append(byVersion, map[string]any{
			"version":     version,
			"hours":       roundFloat(hours, 2),
			"issue_count": len(versionIssues[version]),
		})
	}

	sort.Slice(byVersion, func(i, j int) bool {
		return byVersion[i]["hours"].(float64) > byVersion[j]["hours"].(float64)
	})

	result.ByVersion = byVersion
}

// calculateByCustomField aggregates hours by custom fields
func (rg *ReportGenerator) calculateByCustomField(result *ProjectAnalysisResult, entries []redmine.TimeEntry, issues []redmine.Issue, issueMap map[int]redmine.Issue, filterFields []string) {
	// Build custom field mapping: issue_id -> field_name -> value
	issueCFMap := make(map[int]map[string]string)
	allFieldNames := make(map[string]bool)

	for _, issue := range issues {
		issueCFMap[issue.ID] = make(map[string]string)
		for _, cf := range issue.CustomFields {
			value := ""
			if cf.Value != nil {
				switch v := cf.Value.(type) {
				case string:
					value = v
				case []interface{}:
					// Multi-value field
					var vals []string
					for _, item := range v {
						if s, ok := item.(string); ok {
							vals = append(vals, s)
						}
					}
					value = strings.Join(vals, ", ")
				}
			}
			if value != "" {
				issueCFMap[issue.ID][cf.Name] = value
				allFieldNames[cf.Name] = true
			}
		}
	}

	// Filter fields if specified
	fieldsToAnalyze := make(map[string]bool)
	if len(filterFields) > 0 {
		for _, f := range filterFields {
			fieldsToAnalyze[strings.TrimSpace(f)] = true
		}
	} else {
		fieldsToAnalyze = allFieldNames
	}

	// Aggregate by each custom field
	for fieldName := range fieldsToAnalyze {
		if !allFieldNames[fieldName] {
			continue
		}

		valueHours := make(map[string]float64)
		valueIssues := make(map[string]map[int]bool)
		var totalHours float64

		for _, entry := range entries {
			if entry.Issue == nil {
				continue
			}
			if cfMap, ok := issueCFMap[entry.Issue.ID]; ok {
				value := cfMap[fieldName]
				if value == "" {
					value = "(none)"
				}
				valueHours[value] += entry.Hours
				totalHours += entry.Hours
				if valueIssues[value] == nil {
					valueIssues[value] = make(map[int]bool)
				}
				valueIssues[value][entry.Issue.ID] = true
			}
		}

		var byValue []map[string]any
		for value, hours := range valueHours {
			issueCount := len(valueIssues[value])
			avgHours := 0.0
			if issueCount > 0 {
				avgHours = hours / float64(issueCount)
			}
			percentage := 0.0
			if totalHours > 0 {
				percentage = (hours / totalHours) * 100
			}
			byValue = append(byValue, map[string]any{
				"value":       value,
				"hours":       roundFloat(hours, 2),
				"issue_count": issueCount,
				"avg_hours":   roundFloat(avgHours, 2),
				"percentage":  roundFloat(percentage, 2),
			})
		}

		sort.Slice(byValue, func(i, j int) bool {
			return byValue[i]["hours"].(float64) > byValue[j]["hours"].(float64)
		})

		if len(byValue) > 0 {
			result.ByCustomField[fieldName] = byValue
		}
	}
}

// calculateMonthlyTrend aggregates hours by month
func (rg *ReportGenerator) calculateMonthlyTrend(result *ProjectAnalysisResult, entries []redmine.TimeEntry) {
	monthHours := make(map[string]float64)
	monthIssues := make(map[string]map[int]bool)

	for _, entry := range entries {
		// Parse spent_on date
		t, err := time.Parse("2006-01-02", entry.SpentOn)
		if err != nil {
			continue
		}
		month := t.Format("2006-01")
		monthHours[month] += entry.Hours
		if entry.Issue != nil {
			if monthIssues[month] == nil {
				monthIssues[month] = make(map[int]bool)
			}
			monthIssues[month][entry.Issue.ID] = true
		}
	}

	var monthlyTrend []map[string]any
	for month, hours := range monthHours {
		monthlyTrend = append(monthlyTrend, map[string]any{
			"month":       month,
			"hours":       roundFloat(hours, 2),
			"issue_count": len(monthIssues[month]),
		})
	}

	sort.Slice(monthlyTrend, func(i, j int) bool {
		return monthlyTrend[i]["month"].(string) < monthlyTrend[j]["month"].(string)
	})

	result.MonthlyTrend = monthlyTrend
}

// calculateTopIssues finds the top 20 issues by hours spent
func (rg *ReportGenerator) calculateTopIssues(result *ProjectAnalysisResult, entries []redmine.TimeEntry, issueMap map[int]redmine.Issue) {
	issueHours := make(map[int]float64)
	var unlinkedHours float64

	for _, entry := range entries {
		if entry.Issue == nil {
			unlinkedHours += entry.Hours
			continue
		}
		issueHours[entry.Issue.ID] += entry.Hours
	}

	result.UnlinkedHours = roundFloat(unlinkedHours, 2)

	type issueWithHours struct {
		ID    int
		Hours float64
	}

	var sortedIssues []issueWithHours
	for id, hours := range issueHours {
		sortedIssues = append(sortedIssues, issueWithHours{ID: id, Hours: hours})
	}

	sort.Slice(sortedIssues, func(i, j int) bool {
		return sortedIssues[i].Hours > sortedIssues[j].Hours
	})

	// Take top 20
	limit := 20
	if len(sortedIssues) < limit {
		limit = len(sortedIssues)
	}

	var topIssues []map[string]any
	for i := 0; i < limit; i++ {
		issueID := sortedIssues[i].ID
		hours := sortedIssues[i].Hours
		if issue, ok := issueMap[issueID]; ok {
			topIssues = append(topIssues, map[string]any{
				"id":      issueID,
				"subject": issue.Subject,
				"tracker": issue.Tracker.Name,
				"status":  issue.Status.Name,
				"hours":   roundFloat(hours, 2),
			})
		}
	}

	result.TopIssues = topIssues
}

// GenerateCSV generates a CSV representation of the analysis result
func (rg *ReportGenerator) GenerateCSV(result *ProjectAnalysisResult) string {
	var sb strings.Builder

	// Summary section
	sb.WriteString("=== Project Analysis Report ===\n")
	sb.WriteString(fmt.Sprintf("Project,%s\n", result.Project["name"]))
	if result.Period != "" {
		sb.WriteString(fmt.Sprintf("Period,%s\n", result.Period))
	}
	sb.WriteString("\n")

	sb.WriteString("=== Summary ===\n")
	sb.WriteString("Metric,Value\n")
	for key, value := range result.Summary {
		sb.WriteString(fmt.Sprintf("%s,%v\n", key, value))
	}
	sb.WriteString("\n")

	// By Tracker
	sb.WriteString("=== By Tracker ===\n")
	sb.WriteString("Tracker,Hours,Issue Count,Avg Hours\n")
	for _, item := range result.ByTracker {
		sb.WriteString(fmt.Sprintf("%s,%v,%v,%v\n", item["tracker"], item["hours"], item["issue_count"], item["avg_hours"]))
	}
	sb.WriteString("\n")

	// By User
	sb.WriteString("=== By User ===\n")
	sb.WriteString("User,Hours,Issue Count\n")
	for _, item := range result.ByUser {
		sb.WriteString(fmt.Sprintf("%s,%v,%v\n", item["user"], item["hours"], item["issue_count"]))
	}
	sb.WriteString("\n")

	// Monthly Trend
	sb.WriteString("=== Monthly Trend ===\n")
	sb.WriteString("Month,Hours,Issue Count\n")
	for _, item := range result.MonthlyTrend {
		sb.WriteString(fmt.Sprintf("%s,%v,%v\n", item["month"], item["hours"], item["issue_count"]))
	}

	return sb.String()
}

// AttachResult handles attaching the generated file to Redmine
func (rg *ReportGenerator) AttachResult(content []byte, filename string, params ProjectAnalysisParams) (string, error) {
	attachTo := params.AttachTo
	if attachTo == "" {
		return "", nil // Return base64 instead
	}

	// Upload file first
	// DMSF uses its own upload API, handle separately
	if attachTo == "dmsf" || strings.HasPrefix(attachTo, "dmsf:") {
		return rg.attachToDMSF(content, filename, params, attachTo)
	}

	// Standard Redmine upload for other targets
	token, err := rg.client.UploadFile(filename, bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	// Parse attach_to
	switch {
	case attachTo == "files" || strings.HasPrefix(attachTo, "files:"):
		return rg.attachToFiles(token, filename, params, attachTo)
	case strings.HasPrefix(attachTo, "issue:"):
		return rg.attachToIssue(token, filename, params, attachTo)
	default:
		return "", fmt.Errorf("invalid attach_to value: %s (valid: dmsf, dmsf:FolderID, files, files:VersionName, issue:ID)", attachTo)
	}
}

func (rg *ReportGenerator) attachToFiles(token *redmine.UploadToken, filename string, params ProjectAnalysisParams, attachTo string) (string, error) {
	fileParams := redmine.CreateProjectFileParams{
		ProjectID:   params.ProjectID,
		Token:       token.Token,
		Filename:    filename,
		Description: fmt.Sprintf("Project analysis report generated on %s", time.Now().Format("2006-01-02 15:04:05")),
	}

	// Check for version specification
	if strings.HasPrefix(attachTo, "files:") {
		versionName := strings.TrimPrefix(attachTo, "files:")
		versionID, err := rg.resolver.ResolveVersion(versionName, params.ProjectID)
		if err != nil {
			return "", fmt.Errorf("failed to resolve version '%s': %w", versionName, err)
		}
		fileParams.VersionID = versionID
	}

	file, err := rg.client.CreateProjectFile(fileParams)
	if err != nil {
		return "", fmt.Errorf("failed to create project file: %w", err)
	}

	return file.ContentURL, nil
}

func (rg *ReportGenerator) attachToIssue(token *redmine.UploadToken, filename string, params ProjectAnalysisParams, attachTo string) (string, error) {
	issueIDStr := strings.TrimPrefix(attachTo, "issue:")
	issueID, err := strconv.Atoi(issueIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid issue ID: %s", issueIDStr)
	}

	// Update issue with attachment
	updateParams := redmine.UpdateIssueParams{
		IssueID: issueID,
		Notes:   fmt.Sprintf("Project analysis report attached (generated on %s)", time.Now().Format("2006-01-02 15:04:05")),
		Uploads: []redmine.UploadToken{*token},
	}

	if err := rg.client.UpdateIssue(updateParams); err != nil {
		return "", fmt.Errorf("failed to attach to issue: %w", err)
	}

	return fmt.Sprintf("/issues/%d", issueID), nil
}

func (rg *ReportGenerator) attachToDMSF(content []byte, filename string, params ProjectAnalysisParams, attachTo string) (string, error) {
	// Parse folder ID if specified (dmsf:folder_id)
	var folderID int
	if strings.HasPrefix(attachTo, "dmsf:") {
		folderIDStr := strings.TrimPrefix(attachTo, "dmsf:")
		var err error
		folderID, err = strconv.Atoi(folderIDStr)
		if err != nil {
			return "", fmt.Errorf("invalid DMSF folder ID: %s", folderIDStr)
		}
	}

	// Use DMSF API to upload and commit
	dmsfParams := redmine.DMSFCreateFileParams{
		ProjectID:   params.ProjectID,
		Filename:    filename,
		Title:       fmt.Sprintf("Project Analysis Report - %s", time.Now().Format("2006-01-02")),
		Description: fmt.Sprintf("Project analysis report generated on %s", time.Now().Format("2006-01-02 15:04:05")),
		Comment:     "Uploaded via MCP",
		FolderID:    folderID,
		Content:     bytes.NewReader(content),
	}

	file, err := rg.client.DMSFCreateFile(dmsfParams)
	if err != nil {
		return "", fmt.Errorf("failed to create DMSF file: %w", err)
	}

	// Return the full DMSF file download URL
	return fmt.Sprintf("%s/dmsf/files/%d/%s", rg.client.BaseURL(), file.ID, filename), nil
}

// Helper functions

func roundFloat(val float64, precision int) float64 {
	ratio := float64(1)
	for i := 0; i < precision; i++ {
		ratio *= 10
	}
	return float64(int(val*ratio+0.5)) / ratio
}

