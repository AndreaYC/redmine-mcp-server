package mcp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
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
	Format       string   // "json", "csv", "excel"
	AttachTo     string   // "files", "files:v1.0", "wiki", "wiki:PageName", "issue:123"
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

// ProjectsCompareParams are parameters for multi-project comparison
type ProjectsCompareParams struct {
	Projects      []struct{ ID int; Name string }
	From          string
	To            string
	IssueStatus   string
	Format        string
	AttachTo      string
	TargetProject int
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
		if isClosedStatus(issue.Status.Name) {
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

// GenerateExcel generates an Excel file from the analysis result
func (rg *ReportGenerator) GenerateExcel(result *ProjectAnalysisResult) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Sheet 1: Summary
	rg.createSummarySheet(f, result)

	// Sheet 2: By Tracker
	rg.createByTrackerSheet(f, result)

	// Sheet 3: By User
	rg.createByUserSheet(f, result)

	// Sheet 4: By Version
	rg.createByVersionSheet(f, result)

	// Sheet 5: By Custom Field
	rg.createByCustomFieldSheet(f, result)

	// Sheet 6: Monthly Trend
	rg.createMonthlyTrendSheet(f, result)

	// Sheet 7: Top Issues
	rg.createTopIssuesSheet(f, result)

	// Delete default Sheet1
	_ = f.DeleteSheet("Sheet1")

	// Write to buffer
	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		return nil, fmt.Errorf("failed to write Excel file: %w", err)
	}

	return buf.Bytes(), nil
}

func (rg *ReportGenerator) createSummarySheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "Summary"
	_, _ = f.NewSheet(sheet)

	// Title
	_ = f.SetCellValue(sheet, "A1", "Project Analysis Report")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("Project: %s", result.Project["name"]))
	if result.Period != "" {
		_ = f.SetCellValue(sheet, "A3", fmt.Sprintf("Period: %s", result.Period))
	}

	// Summary data
	row := 5
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Metric")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Value")
	row++

	summaryItems := []struct{ key, label string }{
		{"total_hours", "Total Hours"},
		{"person_days", "Person Days"},
		{"total_issues", "Total Issues"},
		{"closed_issues", "Closed Issues"},
		{"open_issues", "Open Issues"},
		{"contributors", "Contributors"},
		{"avg_hours_per_person", "Avg Hours/Person"},
	}

	for _, item := range summaryItems {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item.label)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), result.Summary[item.key])
		row++
	}

	if result.UnlinkedHours > 0 {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Unlinked Hours")
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), result.UnlinkedHours)
	}
}

func (rg *ReportGenerator) createByTrackerSheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "By Tracker"
	_, _ = f.NewSheet(sheet)

	headers := []string{"Tracker", "Hours", "Issue Count", "Avg Hours/Issue"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, fmt.Sprintf("%c1", 'A'+i), h)
	}

	for i, item := range result.ByTracker {
		row := i + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item["tracker"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item["hours"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item["issue_count"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), item["avg_hours"])
	}
}

func (rg *ReportGenerator) createByUserSheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "By User"
	_, _ = f.NewSheet(sheet)

	headers := []string{"User", "Hours", "Issue Count"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, fmt.Sprintf("%c1", 'A'+i), h)
	}

	for i, item := range result.ByUser {
		row := i + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item["user"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item["hours"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item["issue_count"])
	}
}

func (rg *ReportGenerator) createByVersionSheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "By Version"
	_, _ = f.NewSheet(sheet)

	headers := []string{"Version", "Hours", "Issue Count"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, fmt.Sprintf("%c1", 'A'+i), h)
	}

	for i, item := range result.ByVersion {
		row := i + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item["version"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item["hours"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item["issue_count"])
	}
}

func (rg *ReportGenerator) createByCustomFieldSheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "By Custom Field"
	_, _ = f.NewSheet(sheet)

	row := 1
	for fieldName, values := range result.ByCustomField {
		// Field header
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), fieldName)
		row++

		// Column headers
		headers := []string{"Value", "Hours", "Issue Count", "Avg Hours", "Percentage"}
		for i, h := range headers {
			_ = f.SetCellValue(sheet, fmt.Sprintf("%c%d", 'A'+i, row), h)
		}
		row++

		// Data
		for _, item := range values {
			_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item["value"])
			_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item["hours"])
			_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item["issue_count"])
			_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), item["avg_hours"])
			_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), fmt.Sprintf("%.1f%%", item["percentage"]))
			row++
		}

		row++ // Empty row between fields
	}
}

func (rg *ReportGenerator) createMonthlyTrendSheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "Monthly Trend"
	_, _ = f.NewSheet(sheet)

	headers := []string{"Month", "Hours", "Issue Count"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, fmt.Sprintf("%c1", 'A'+i), h)
	}

	for i, item := range result.MonthlyTrend {
		row := i + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item["month"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item["hours"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item["issue_count"])
	}
}

func (rg *ReportGenerator) createTopIssuesSheet(f *excelize.File, result *ProjectAnalysisResult) {
	sheet := "Top Issues"
	_, _ = f.NewSheet(sheet)

	headers := []string{"ID", "Subject", "Tracker", "Status", "Hours"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, fmt.Sprintf("%c1", 'A'+i), h)
	}

	for i, item := range result.TopIssues {
		row := i + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), item["id"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), item["subject"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), item["tracker"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), item["status"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), item["hours"])
	}
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
	case attachTo == "wiki" || strings.HasPrefix(attachTo, "wiki:"):
		return rg.attachToWiki(token, filename, params, attachTo)
	case strings.HasPrefix(attachTo, "issue:"):
		return rg.attachToIssue(token, filename, params, attachTo)
	default:
		return "", fmt.Errorf("invalid attach_to value: %s (valid: dmsf, files, wiki, issue:ID)", attachTo)
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

func (rg *ReportGenerator) attachToWiki(token *redmine.UploadToken, filename string, params ProjectAnalysisParams, attachTo string) (string, error) {
	pageTitle := "Reports"
	if strings.HasPrefix(attachTo, "wiki:") {
		pageTitle = strings.TrimPrefix(attachTo, "wiki:")
	}

	// Get or create wiki page
	wikiPage, err := rg.client.GetWikiPage(params.ProjectID, pageTitle)
	if err != nil {
		// Page doesn't exist, create it
		err = rg.client.CreateOrUpdateWikiPage(redmine.WikiPageParams{
			ProjectID: params.ProjectID,
			Title:     pageTitle,
			Text:      fmt.Sprintf("h1. %s\n\nProject analysis reports are attached to this page.", pageTitle),
			Comments:  "Created by MCP report generator",
		})
		if err != nil {
			return "", fmt.Errorf("failed to create wiki page: %w", err)
		}
	}

	// Update wiki page with attachment
	currentText := ""
	if wikiPage != nil {
		currentText = wikiPage.Text
	}

	newText := currentText + fmt.Sprintf("\n\n---\n*Report generated on %s*\nattachment:%s",
		time.Now().Format("2006-01-02 15:04:05"), filename)

	err = rg.client.CreateOrUpdateWikiPage(redmine.WikiPageParams{
		ProjectID: params.ProjectID,
		Title:     pageTitle,
		Text:      newText,
		Comments:  "Added analysis report",
	})
	if err != nil {
		return "", fmt.Errorf("failed to update wiki page: %w", err)
	}

	// Note: Wiki attachment upload requires different API handling
	// For now, return a reference to the wiki page
	return fmt.Sprintf("/projects/%d/wiki/%s", params.ProjectID, pageTitle), nil
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

func isClosedStatus(status string) bool {
	closedStatuses := []string{"Closed", "Rejected", "Resolved", "Done", "關閉", "已解決", "已拒絕"}
	for _, s := range closedStatuses {
		if strings.EqualFold(status, s) {
			return true
		}
	}
	return false
}

func roundFloat(val float64, precision int) float64 {
	ratio := float64(1)
	for i := 0; i < precision; i++ {
		ratio *= 10
	}
	return float64(int(val*ratio+0.5)) / ratio
}

// ToBase64 encodes bytes to base64 string
func ToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// GenerateComparisonExcel generates an Excel file comparing multiple projects
func (rg *ReportGenerator) GenerateComparisonExcel(projects []map[string]any) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Sheet 1: Summary Comparison
	sheet := "Summary Comparison"
	_, _ = f.NewSheet(sheet)

	// Headers
	headers := []string{"Project", "Total Hours", "Person Days", "Total Issues", "Closed Issues", "Contributors", "Avg Hours/Person"}
	for i, h := range headers {
		_ = f.SetCellValue(sheet, fmt.Sprintf("%c1", 'A'+i), h)
	}

	// Data rows
	for i, p := range projects {
		row := i + 2
		proj := p["project"].(map[string]any)
		summary := p["summary"].(map[string]any)

		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), proj["name"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), summary["total_hours"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), summary["person_days"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), summary["total_issues"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), summary["closed_issues"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), summary["contributors"])
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), summary["avg_hours_per_person"])
	}

	// Sheet 2: By Tracker Comparison
	sheet2 := "By Tracker"
	_, _ = f.NewSheet(sheet2)

	row := 1
	for _, p := range projects {
		proj := p["project"].(map[string]any)
		byTracker, ok := p["by_tracker"].([]map[string]any)
		if !ok {
			continue
		}

		_ = f.SetCellValue(sheet2, fmt.Sprintf("A%d", row), proj["name"])
		row++

		_ = f.SetCellValue(sheet2, fmt.Sprintf("A%d", row), "Tracker")
		_ = f.SetCellValue(sheet2, fmt.Sprintf("B%d", row), "Hours")
		_ = f.SetCellValue(sheet2, fmt.Sprintf("C%d", row), "Issue Count")
		_ = f.SetCellValue(sheet2, fmt.Sprintf("D%d", row), "Avg Hours")
		row++

		for _, t := range byTracker {
			_ = f.SetCellValue(sheet2, fmt.Sprintf("A%d", row), t["tracker"])
			_ = f.SetCellValue(sheet2, fmt.Sprintf("B%d", row), t["hours"])
			_ = f.SetCellValue(sheet2, fmt.Sprintf("C%d", row), t["issue_count"])
			_ = f.SetCellValue(sheet2, fmt.Sprintf("D%d", row), t["avg_hours"])
			row++
		}
		row++ // Empty row between projects
	}

	// Sheet 3: Monthly Trend Comparison
	sheet3 := "Monthly Trend"
	_, _ = f.NewSheet(sheet3)

	row = 1
	for _, p := range projects {
		proj := p["project"].(map[string]any)
		trend, ok := p["monthly_trend"].([]map[string]any)
		if !ok {
			continue
		}

		_ = f.SetCellValue(sheet3, fmt.Sprintf("A%d", row), proj["name"])
		row++

		_ = f.SetCellValue(sheet3, fmt.Sprintf("A%d", row), "Month")
		_ = f.SetCellValue(sheet3, fmt.Sprintf("B%d", row), "Hours")
		_ = f.SetCellValue(sheet3, fmt.Sprintf("C%d", row), "Issue Count")
		row++

		for _, m := range trend {
			_ = f.SetCellValue(sheet3, fmt.Sprintf("A%d", row), m["month"])
			_ = f.SetCellValue(sheet3, fmt.Sprintf("B%d", row), m["hours"])
			_ = f.SetCellValue(sheet3, fmt.Sprintf("C%d", row), m["issue_count"])
			row++
		}
		row++ // Empty row between projects
	}

	// Delete default Sheet1
	_ = f.DeleteSheet("Sheet1")

	// Write to buffer
	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		return nil, fmt.Errorf("failed to write Excel file: %w", err)
	}

	return buf.Bytes(), nil
}
