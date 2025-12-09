package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReportParser parses markdown report files
type ReportParser struct {
	reportsPath string
	cache       map[string]*ParsedReport
	mu          sync.RWMutex
}

// ParsedReport contains parsed data from a report file
type ParsedReport struct {
	PeriodType      string
	StartTime       time.Time
	EndTime         time.Time
	ScreenshotCount int
	Summary         string
	Analysis        string
	Timestamp       time.Time
	ScreenshotID    string
	ImagePath       string
	ScreenID        int
	HourKey         string
}

// NewReportParser creates a new report parser
func NewReportParser(reportsPath string) *ReportParser {
	return &ReportParser{
		reportsPath: reportsPath,
		cache:       make(map[string]*ParsedReport),
	}
}

// ParsePeriodReport parses a period summary report file (day.md, hour.md, etc.)
func (p *ReportParser) ParsePeriodReport(filePath string) (*ParsedReport, error) {
	// Check cache first with read lock
	p.mu.RLock()
	if cached, ok := p.cache[filePath]; ok {
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	report := &ParsedReport{}
	lines := strings.Split(string(content), "\n")

	var inSummary bool
	var inAnalysis bool
	var summaryLines []string
	var analysisLines []string

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Parse period type
		if strings.HasPrefix(line, "**周期类型**:") {
			report.PeriodType = strings.TrimSpace(strings.TrimPrefix(line, "**周期类型**:"))
		}

		// Parse start time
		if strings.HasPrefix(line, "**开始时间**:") {
			timeStr := strings.TrimSpace(strings.TrimPrefix(line, "**开始时间**:"))
			if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
				report.StartTime = t
			}
		}

		// Parse end time
		if strings.HasPrefix(line, "**结束时间**:") {
			timeStr := strings.TrimSpace(strings.TrimPrefix(line, "**结束时间**:"))
			if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
				report.EndTime = t
			}
		}

		// Parse screenshot count
		if strings.HasPrefix(line, "**截图数量**:") {
			countStr := strings.TrimSpace(strings.TrimPrefix(line, "**截图数量**:"))
			if count, err := strconv.Atoi(countStr); err == nil {
				report.ScreenshotCount = count
			}
		}

		// Parse summary section
		if line == "## 事实总结" {
			inSummary = true
			inAnalysis = false
			continue
		}

		// Parse analysis section
		if line == "## 改进建议" {
			inSummary = false
			inAnalysis = true
			continue
		}

		// Stop at separator or next section
		if line == "---" && (inSummary || inAnalysis) {
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextLine, "##") {
					inSummary = false
					inAnalysis = false
					continue
				}
			}
		}

		// Collect summary content
		if inSummary && line != "" && !strings.HasPrefix(line, "---") {
			summaryLines = append(summaryLines, line)
		}

		// Collect analysis content
		if inAnalysis && line != "" && !strings.HasPrefix(line, "---") {
			analysisLines = append(analysisLines, line)
		}

		// Parse report generation time
		if strings.HasPrefix(line, "*报告生成时间:") {
			timeStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "*报告生成时间:"), "*"))
			if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
				report.Timestamp = t
			}
		}
	}

	report.Summary = strings.Join(summaryLines, "\n")
	report.Analysis = strings.Join(analysisLines, "\n")

	// Validate required fields
	if report.PeriodType == "" {
		return nil, fmt.Errorf("missing period type in report: %s", filePath)
	}
	if report.StartTime.IsZero() {
		return nil, fmt.Errorf("missing or invalid start time in report: %s", filePath)
	}
	if report.EndTime.IsZero() {
		return nil, fmt.Errorf("missing or invalid end time in report: %s", filePath)
	}

	// Cache the result with write lock
	p.mu.Lock()
	p.cache[filePath] = report
	p.mu.Unlock()

	return report, nil
}

// ParseScreenshotReport parses a screenshot-level report file (MM.md)
func (p *ReportParser) ParseScreenshotReport(filePath string) (*ParsedReport, error) {
	// Check cache first with read lock
	p.mu.RLock()
	if cached, ok := p.cache[filePath]; ok {
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file: %w", err)
	}

	report := &ParsedReport{}
	lines := strings.Split(string(content), "\n")

	var inSummary bool
	var summaryLines []string

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Parse timestamp
		if strings.HasPrefix(line, "**时间**:") {
			timeStr := strings.TrimSpace(strings.TrimPrefix(line, "**时间**:"))
			if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
				report.StartTime = t
				report.EndTime = t
				report.Timestamp = t
				report.HourKey = t.Format("2006-01-02-15")
			}
		}

		// Parse screenshot ID
		if strings.HasPrefix(line, "**截图ID**:") {
			report.ScreenshotID = strings.TrimSpace(strings.TrimPrefix(line, "**截图ID**:"))
		}

		// Parse image path
		if strings.HasPrefix(line, "**截图路径**:") {
			report.ImagePath = strings.TrimSpace(strings.TrimPrefix(line, "**截图路径**:"))
		}

		// Parse screen ID
		if strings.HasPrefix(line, "**屏幕ID**:") {
			screenIDStr := strings.TrimSpace(strings.TrimPrefix(line, "**屏幕ID**:"))
			if id, err := strconv.Atoi(screenIDStr); err == nil {
				report.ScreenID = id
			}
		}

		// Parse summary section
		if line == "## 事实总结" {
			inSummary = true
			continue
		}

		// Stop at separator
		if line == "---" && inSummary {
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextLine, "*") {
					inSummary = false
					continue
				}
			}
		}

		// Collect summary content
		if inSummary && line != "" && !strings.HasPrefix(line, "---") {
			summaryLines = append(summaryLines, line)
		}

		// Parse report generation time
		if strings.HasPrefix(line, "*报告生成时间:") {
			timeStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "*报告生成时间:"), "*"))
			if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
				report.Timestamp = t
			}
		}
	}

	report.Summary = strings.Join(summaryLines, "\n")
	report.ScreenshotCount = 1

	// Cache the result with write lock
	p.mu.Lock()
	p.cache[filePath] = report
	p.mu.Unlock()

	return report, nil
}

// ParseReportFile automatically detects report type and parses it
func (p *ReportParser) ParseReportFile(filePath string) (*ParsedReport, error) {
	// Check if it's a screenshot-level report (MM.md format)
	filename := filepath.Base(filePath)

	// Screenshot reports are named like "01.md", "55.md", etc. (2 digits)
	screenshotPattern := regexp.MustCompile(`^\d{2}\.md$`)
	if screenshotPattern.MatchString(filename) {
		return p.ParseScreenshotReport(filePath)
	}

	// Otherwise, it's a period report
	return p.ParsePeriodReport(filePath)
}

// ExtractPeriodKeyFromPath extracts period key from file path
func ExtractPeriodKeyFromPath(filePath string, periodType string) (string, error) {
	relPath, err := filepath.Rel("", filePath)
	if err != nil {
		return "", err
	}

	parts := strings.Split(relPath, string(filepath.Separator))

	switch periodType {
	case "day":
		// reports/2025/12/02/day.md -> 2025-12-02
		if len(parts) >= 3 {
			return fmt.Sprintf("%s-%s-%s", parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1][:2]), nil
		}
	case "hour":
		// reports/2025/12/02/10/hour.md -> 2025-12-02-10
		if len(parts) >= 4 {
			return fmt.Sprintf("%s-%s-%s-%s", parts[len(parts)-4], parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1][:2]), nil
		}
	case "fifteenmin":
		// reports/2025/12/02/14/fifteenmin-15.md -> 2025-12-02-14-15
		if len(parts) >= 5 {
			filename := parts[len(parts)-1]
			hour := parts[len(parts)-2]
			day := parts[len(parts)-3]
			month := parts[len(parts)-4]
			year := parts[len(parts)-5]
			// Extract minute from filename like "fifteenmin-15.md"
			re := regexp.MustCompile(`fifteenmin-(\d+)\.md`)
			matches := re.FindStringSubmatch(filename)
			if len(matches) == 2 {
				return fmt.Sprintf("%s-%s-%s-%s-%s", year, month, day, hour, matches[1]), nil
			}
		}
	case "halfhour":
		// reports/2025/12/02/14/halfhour-30.md -> 2025-12-02-14-30
		if len(parts) >= 5 {
			filename := parts[len(parts)-1]
			hour := parts[len(parts)-2]
			day := parts[len(parts)-3]
			month := parts[len(parts)-4]
			year := parts[len(parts)-5]
			re := regexp.MustCompile(`halfhour-(\d+)\.md`)
			matches := re.FindStringSubmatch(filename)
			if len(matches) == 2 {
				return fmt.Sprintf("%s-%s-%s-%s-%s", year, month, day, hour, matches[1]), nil
			}
		}
	case "work-segment":
		// reports/2025/12/02/work-segment-0.md -> 2025-12-02-work-segment-0
		if len(parts) >= 3 {
			filename := parts[len(parts)-1]
			return strings.TrimSuffix(filename, ".md"), nil
		}
	case "week":
		// reports/2025/12/week-W49.md -> 2025-12-01-week (need to calculate week start)
		if len(parts) >= 3 {
			filename := parts[len(parts)-1]
			re := regexp.MustCompile(`week-W(\d+)\.md`)
			matches := re.FindStringSubmatch(filename)
			if len(matches) == 2 {
				// For now, return a simple key based on filename
				return strings.TrimSuffix(filename, ".md"), nil
			}
		}
	case "month":
		// reports/2025/12/month.md -> 2025-12
		if len(parts) >= 2 {
			return fmt.Sprintf("%s-%s", parts[len(parts)-2], parts[len(parts)-1][:2]), nil
		}
	case "year":
		// reports/2025/year.md -> 2025
		if len(parts) >= 1 {
			return strings.TrimSuffix(parts[len(parts)-1], ".md"), nil
		}
	}

	return "", fmt.Errorf("unable to extract period key from path: %s", filePath)
}

// ValidatePeriodKeyFromStartTime validates that a period_key matches the start_time
// This ensures consistency between file path and file content
func ValidatePeriodKeyFromStartTime(periodKey string, periodType string, startTime time.Time) error {
	var expectedKey string

	switch periodType {
	case "fifteenmin":
		expectedKey = startTime.Format("2006-01-02-15-04")
	case "hour":
		expectedKey = startTime.Format("2006-01-02-15")
	case "day":
		expectedKey = startTime.Format("2006-01-02")
	case "work-segment":
		// Work-segment keys include segment number, so we only validate date part
		if strings.HasPrefix(periodKey, startTime.Format("2006-01-02")) {
			return nil
		}
		return fmt.Errorf("period_key %s does not match start_time %s for work-segment", periodKey, startTime.Format("2006-01-02"))
	case "week":
		// Week keys are YYYY-MM-DD-week format, where date is Monday
		weekday := int(startTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := startTime.AddDate(0, 0, -(weekday - 1))
		expectedKey = monday.Format("2006-01-02") + "-week"
	case "month":
		expectedKey = startTime.Format("2006-01")
	case "quarter":
		quarter := (int(startTime.Month())-1)/3 + 1
		expectedKey = fmt.Sprintf("%d-Q%d", startTime.Year(), quarter)
	case "year":
		expectedKey = startTime.Format("2006")
	default:
		// For unknown types, skip validation
		return nil
	}

	if periodKey != expectedKey {
		return fmt.Errorf("period_key mismatch: expected %s (from start_time %s), got %s", expectedKey, startTime.Format("2006-01-02 15:04:05"), periodKey)
	}

	return nil
}

// BuildPeriodKeyFromStartTime builds period_key from start_time and period_type
// This is used to validate or correct period_key extracted from file path
func BuildPeriodKeyFromStartTime(startTime time.Time, periodType string) string {
	switch periodType {
	case "fifteenmin":
		return startTime.Format("2006-01-02-15-04")
	case "hour":
		return startTime.Format("2006-01-02-15")
	case "day":
		return startTime.Format("2006-01-02")
	case "week":
		weekday := int(startTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := startTime.AddDate(0, 0, -(weekday - 1))
		return monday.Format("2006-01-02") + "-week"
	case "month":
		return startTime.Format("2006-01")
	case "quarter":
		quarter := (int(startTime.Month())-1)/3 + 1
		return fmt.Sprintf("%d-Q%d", startTime.Year(), quarter)
	case "year":
		return startTime.Format("2006")
	default:
		return ""
	}
}

// ClearCache clears the parser cache
func (p *ReportParser) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*ParsedReport)
}

// ClearCacheForFile clears cache for a specific file
func (p *ReportParser) ClearCacheForFile(filePath string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cache, filePath)
}
