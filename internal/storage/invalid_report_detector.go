package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// InvalidReportIssue represents an issue found in a report
type InvalidReportIssue struct {
	FilePath string
	Issue    string
	Category string // "parse_error", "content_invalid", "path_mismatch", "logic_error"
}

// DetectInvalidReports scans report files and detects invalid ones
// Returns a list of issues found
func DetectInvalidReports(reportsPath string) ([]InvalidReportIssue, error) {
	var issues []InvalidReportIssue

	// Walk through all .md files in reports directory
	err := filepath.Walk(reportsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only check .md files
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Skip screenshot-level reports (MM.md format)
		filename := filepath.Base(path)
		screenshotPattern := regexp.MustCompile(`^\d{2}\.md$`)
		if screenshotPattern.MatchString(filename) {
			return nil
		}

		// Detect issues for this file
		fileIssues := detectIssuesInFile(reportsPath, path)
		issues = append(issues, fileIssues...)

		return nil
	})

	return issues, err
}

// detectIssuesInFile detects all issues in a single report file
func detectIssuesInFile(reportsPath string, filePath string) []InvalidReportIssue {
	var issues []InvalidReportIssue

	parser := NewReportParser(reportsPath)

	// Try to parse the file
	parsed, err := parser.ParsePeriodReport(filePath)
	if err != nil {
		issues = append(issues, InvalidReportIssue{
			FilePath: filePath,
			Issue:    fmt.Sprintf("Failed to parse: %v", err),
			Category: "parse_error",
		})
		return issues // Can't continue if parsing fails
	}

	// Check required fields
	if parsed.PeriodType == "" {
		issues = append(issues, InvalidReportIssue{
			FilePath: filePath,
			Issue:    "Missing period type",
			Category: "parse_error",
		})
	}
	if parsed.StartTime.IsZero() {
		issues = append(issues, InvalidReportIssue{
			FilePath: filePath,
			Issue:    "Missing or invalid start time",
			Category: "parse_error",
		})
	}
	if parsed.EndTime.IsZero() {
		issues = append(issues, InvalidReportIssue{
			FilePath: filePath,
			Issue:    "Missing or invalid end time",
			Category: "parse_error",
		})
	}

	// Infer period type from file path if not available
	periodType := parsed.PeriodType
	if periodType == "" {
		periodType = inferPeriodTypeFromPath(filePath)
	}

	// Check path consistency if we have period type and start time
	if periodType != "" && !parsed.StartTime.IsZero() {
		periodKey, err := ExtractPeriodKeyFromPath(filePath, periodType)
		if err == nil {
			if err := ValidatePeriodKeyFromStartTime(periodKey, periodType, parsed.StartTime); err != nil {
				issues = append(issues, InvalidReportIssue{
					FilePath: filePath,
					Issue:    fmt.Sprintf("Path mismatch: %v", err),
					Category: "path_mismatch",
				})
			}
		}
	}

	// Check content validity
	summary := &PeriodSummary{
		PeriodKey:  "", // Not needed for validation
		PeriodType: parsed.PeriodType,
		StartTime:  parsed.StartTime,
		EndTime:    parsed.EndTime,
		Summary:    parsed.Summary,
		Analysis:   parsed.Analysis,
	}

	if !hasValidReportContent(summary) {
		issues = append(issues, InvalidReportIssue{
			FilePath: filePath,
			Issue:    "Invalid content: summary and analysis are both invalid",
			Category: "content_invalid",
		})
	}

	// Check new logic rules
	// Rule 1: Screenshot count is 0 but has summary
	if parsed.ScreenshotCount == 0 && strings.TrimSpace(parsed.Summary) != "" {
		// Check if summary is not just a placeholder
		if parsed.Summary != "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
			issues = append(issues, InvalidReportIssue{
				FilePath: filePath,
				Issue:    "Logic error: screenshot count is 0 but has fact summary",
				Category: "logic_error",
			})
		}
	}

	// Rule 2: No summary but has analysis
	if strings.TrimSpace(parsed.Summary) == "" && strings.TrimSpace(parsed.Analysis) != "" {
		issues = append(issues, InvalidReportIssue{
			FilePath: filePath,
			Issue:    "Logic error: no fact summary but has improvement suggestions",
			Category: "logic_error",
		})
	}

	return issues
}

// inferPeriodTypeFromPath tries to infer period type from file path
func inferPeriodTypeFromPath(filePath string) string {
	filename := filepath.Base(filePath)
	dir := filepath.Dir(filePath)

	// Check filename patterns
	if strings.HasPrefix(filename, "fifteenmin-") {
		return "fifteenmin"
	}
	if strings.HasPrefix(filename, "halfhour-") {
		return "halfhour"
	}
	if strings.HasPrefix(filename, "work-segment-") {
		return "work-segment"
	}
	if strings.HasPrefix(filename, "week-") {
		return "week"
	}
	if filename == "day.md" {
		return "day"
	}
	if filename == "hour.md" {
		return "hour"
	}
	if filename == "month.md" {
		return "month"
	}
	if filename == "year.md" {
		return "year"
	}

	// Try to infer from directory structure
	parts := strings.Split(dir, string(filepath.Separator))
	// reports/YYYY/MM/DD/HH/hour.md -> hour
	// reports/YYYY/MM/DD/day.md -> day
	// reports/YYYY/MM/month.md -> month
	// reports/YYYY/year.md -> year

	if len(parts) >= 4 {
		// Has hour directory, likely hour or fifteenmin/halfhour
		return "hour" // Default to hour
	}
	if len(parts) >= 3 {
		// Has day directory, likely day
		return "day"
	}
	if len(parts) >= 2 {
		// Has month directory, likely month
		return "month"
	}

	return "" // Unknown
}

// hasValidReportContent checks if a report has valid content
// This is an enhanced version that includes the new logic rules
func hasValidReportContent(summary *PeriodSummary) bool {
	// Check for placeholder marker
	if summary.Summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
		return false
	}

	// Check Summary
	summaryValid := false
	if summary.Summary != "" {
		summaryText := strings.TrimSpace(summary.Summary)
		normalizedText := strings.ToLower(summaryText)
		normalizedText = strings.ReplaceAll(normalizedText, "\n", " ")
		normalizedText = strings.ReplaceAll(normalizedText, "\r", " ")
		normalizedText = strings.ReplaceAll(normalizedText, "  ", " ")
		normalizedText = strings.TrimSpace(normalizedText)

		noActivityPatterns := []string{
			"没有检测到有效工作活动",
			"没有有效工作活动",
			"no work activity",
			"no work activity in segment",
		}

		isOnlyNoActivity := false
		for _, pattern := range noActivityPatterns {
			patternLower := strings.ToLower(pattern)
			if strings.Contains(normalizedText, patternLower) {
				remaining := strings.ReplaceAll(normalizedText, patternLower, "")
				remaining = strings.ReplaceAll(remaining, "（", "")
				remaining = strings.ReplaceAll(remaining, "）", "")
				remaining = strings.ReplaceAll(remaining, "(", "")
				remaining = strings.ReplaceAll(remaining, ")", "")
				remaining = strings.ReplaceAll(remaining, "。", "")
				remaining = strings.ReplaceAll(remaining, ".", "")
				remaining = strings.ReplaceAll(remaining, "，", "")
				remaining = strings.ReplaceAll(remaining, ",", "")
				remaining = strings.ReplaceAll(remaining, " ", "")
				remaining = strings.TrimSpace(remaining)

				if len(remaining) < 50 {
					isOnlyNoActivity = true
					break
				}
			}
		}

		if !isOnlyNoActivity {
			summaryValid = true
		}
	}

	// Check Analysis
	analysisValid := false
	if summary.Analysis != "" {
		analysisText := strings.TrimSpace(summary.Analysis)
		if strings.HasPrefix(analysisText, "分析失败") ||
			strings.HasPrefix(analysisText, "分析失败:") ||
			strings.Contains(analysisText, "API call failed") ||
			strings.Contains(analysisText, "failed to") {
			analysisValid = false
		} else {
			analysisValid = true
		}
	}

	// Report is valid if at least one section has valid content
	return summaryValid || analysisValid
}
