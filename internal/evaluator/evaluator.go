package evaluator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"stuff-time/internal/analyzer"
	"stuff-time/internal/logger"
	"stuff-time/internal/storage"
)

type Evaluator struct {
	analyzer                            *analyzer.OpenAI
	evaluationPromptTemplate            string
	reportContentTemplate               string
	screenshotSourceTemplate            string
	reportFormatTemplate                string
	screenshotSourceSectionTemplate     string
	improvementPromptTemplate           *template.Template
	improvementScreenshotSourceTemplate *template.Template
}

func NewEvaluator(analyzer *analyzer.OpenAI, evaluationPromptTemplate, reportContentTemplate, screenshotSourceTemplate, reportFormatTemplate, screenshotSourceSectionTemplate string) *Evaluator {
	return &Evaluator{
		analyzer:                        analyzer,
		evaluationPromptTemplate:        evaluationPromptTemplate,
		reportContentTemplate:           reportContentTemplate,
		screenshotSourceTemplate:        screenshotSourceTemplate,
		reportFormatTemplate:            reportFormatTemplate,
		screenshotSourceSectionTemplate: screenshotSourceSectionTemplate,
		improvementPromptTemplate:       nil,
	}
}

func NewEvaluatorWithImprovement(analyzer *analyzer.OpenAI, evaluationPromptTemplate, reportContentTemplate, screenshotSourceTemplate, reportFormatTemplate, screenshotSourceSectionTemplate, improvementPromptTemplate, improvementScreenshotSourceTemplate string) (*Evaluator, error) {
	eval := &Evaluator{
		analyzer:                        analyzer,
		evaluationPromptTemplate:        evaluationPromptTemplate,
		reportContentTemplate:           reportContentTemplate,
		screenshotSourceTemplate:        screenshotSourceTemplate,
		reportFormatTemplate:            reportFormatTemplate,
		screenshotSourceSectionTemplate: screenshotSourceSectionTemplate,
	}

	// Parse improvement prompt template
	if improvementPromptTemplate != "" {
		tmpl, err := template.New("improvement").Parse(improvementPromptTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse improvement prompt template: %w", err)
		}
		eval.improvementPromptTemplate = tmpl
	}

	// Parse improvement screenshot source template
	if improvementScreenshotSourceTemplate != "" {
		tmpl, err := template.New("improvementScreenshotSource").Parse(improvementScreenshotSourceTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse improvement screenshot source template: %w", err)
		}
		eval.improvementScreenshotSourceTemplate = tmpl
	}

	return eval, nil
}

func (e *Evaluator) EvaluateReport(summary *storage.PeriodSummary, screenshotRecords map[string]*storage.ScreenshotRecord) (string, error) {
	if summary == nil {
		return "", fmt.Errorf("summary is nil")
	}

	// Build evaluation prompt with screenshot source information
	evaluationPrompt := e.buildEvaluationPrompt(summary, screenshotRecords)

	// Use analysis model for evaluation (same as behavior analysis)
	req := analyzer.VisionRequest{
		Model:               e.analyzer.AnalysisModel,
		MaxCompletionTokens: e.analyzer.MaxCompletionTokens,
		Messages: []analyzer.Message{
			{
				Role: "user",
				Content: []analyzer.ContentObject{
					{
						Type: "text",
						Text: evaluationPrompt,
					},
				},
			},
		},
	}

	evaluationResult, err := e.callAPI(req)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate report: %w", err)
	}

	// Format evaluation report with screenshot source information
	report := e.formatEvaluationReport(summary, evaluationResult, screenshotRecords)
	return report, nil
}

func (e *Evaluator) callAPI(req analyzer.VisionRequest) (string, error) {
	const maxRetries = 3
	const initialBackoff = 2 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s
			backoff := initialBackoff * time.Duration(1<<uint(attempt-1))
			logger.GetLogger().Infof("Retrying API request (attempt %d/%d, backoff: %v)",
				attempt+1, maxRetries+1, backoff)
			time.Sleep(backoff)
		}

		result, err := e.callAPISingle(req)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("API call failed after %d retries: %w", maxRetries+1, lastErr)
}

// isRetryableError checks if an error is retryable (temporary network/server errors)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for retryable HTTP status codes
	retryableStatusCodes := []string{
		"status 502", // Bad Gateway
		"status 503", // Service Unavailable
		"status 504", // Gateway Timeout
		"status 429", // Too Many Requests
		"status 500", // Internal Server Error
	}

	for _, code := range retryableStatusCodes {
		if strings.Contains(errStr, code) {
			return true
		}
	}

	// Check for network errors
	networkErrors := []string{
		"failed to send request",
		"timeout",
		"connection reset",
		"connection refused",
		"no such host",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(strings.ToLower(errStr), netErr) {
			return true
		}
	}

	return false
}

// callAPISingle makes a single API call without retry
func (e *Evaluator) callAPISingle(req analyzer.VisionRequest) (string, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.analyzer.APIKey))

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var visionResp analyzer.VisionResponse
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&visionResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(visionResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := visionResp.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty content in response")
	}

	return content, nil
}

func (e *Evaluator) buildEvaluationPrompt(summary *storage.PeriodSummary, screenshotRecords map[string]*storage.ScreenshotRecord) string {
	screenshotCount := len(strings.Split(summary.Screenshots, ","))
	if summary.Screenshots == "" {
		screenshotCount = 0
	}

	periodTypeName := getPeriodTypeName(summary.PeriodType)

	// Format the template with dynamic values
	prompt := fmt.Sprintf(e.evaluationPromptTemplate,
		periodTypeName,
		summary.StartTime.Format("2006-01-02 15:04:05"),
		summary.EndTime.Format("2006-01-02 15:04:05"),
		screenshotCount)

	// Add report content
	summaryText := summary.Summary
	if summaryText == "" {
		summaryText = "（无内容）"
	}
	analysisText := summary.Analysis
	if analysisText == "" {
		analysisText = "（无内容）"
	}
	reportContent := fmt.Sprintf(e.reportContentTemplate, summaryText, analysisText)
	prompt += "\n\n---\n\n"
	prompt += reportContent
	prompt += "\n\n"

	// Add screenshot source information
	if len(screenshotRecords) > 0 {
		prompt += "---\n\n"
		prompt += e.screenshotSourceTemplate
		prompt += "\n\n"

		// Get screenshot IDs from summary
		screenshotIDs := strings.Split(summary.Screenshots, ",")
		count := 0
		for _, id := range screenshotIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if record, exists := screenshotRecords[id]; exists {
				count++
				if count <= 20 { // Limit to first 20 screenshots to avoid prompt being too long
					prompt += fmt.Sprintf("- **截图 %s** (时间: %s): %s\n",
						id[:8], // Show first 8 chars of ID
						record.Timestamp.Format("2006-01-02 15:04:05"),
						truncateString(record.Analysis, 200))
				}
			}
		}
		if count > 20 {
			prompt += fmt.Sprintf("\n（还有 %d 张截图未列出，但评估时请考虑所有截图）\n", count-20)
		}
		prompt += "\n"
	}

	return prompt
}

func (e *Evaluator) formatEvaluationReport(summary *storage.PeriodSummary, evaluationResult string, screenshotRecords map[string]*storage.ScreenshotRecord) string {
	var sb strings.Builder

	screenshotCount := len(strings.Split(summary.Screenshots, ","))
	if summary.Screenshots == "" {
		screenshotCount = 0
	}

	// Header using template
	header := fmt.Sprintf(e.reportFormatTemplate,
		summary.PeriodType,
		summary.StartTime.Format("2006-01-02 15:04:05"),
		summary.EndTime.Format("2006-01-02 15:04:05"),
		screenshotCount,
		time.Now().Format("2006-01-02 15:04:05"),
		evaluationResult)
	sb.WriteString(header)
	sb.WriteString("\n\n")

	// Add screenshot source information section
	if len(screenshotRecords) > 0 {
		sb.WriteString(e.screenshotSourceSectionTemplate)
		sb.WriteString("\n\n")

		screenshotIDs := strings.Split(summary.Screenshots, ",")
		validCount := 0
		for _, id := range screenshotIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if record, exists := screenshotRecords[id]; exists {
				validCount++
				sb.WriteString(fmt.Sprintf("### 截图 %s\n\n", id[:8]))
				sb.WriteString(fmt.Sprintf("- **完整ID**: %s\n", id))
				sb.WriteString(fmt.Sprintf("- **时间**: %s\n", record.Timestamp.Format("2006-01-02 15:04:05")))
				sb.WriteString(fmt.Sprintf("- **截图路径**: %s\n", record.ImagePath))
				if record.Analysis != "" {
					sb.WriteString(fmt.Sprintf("- **分析内容**: %s\n", truncateString(record.Analysis, 300)))
				}
				sb.WriteString("\n")
			}
		}
		if validCount < screenshotCount {
			sb.WriteString(fmt.Sprintf("**注意**: 部分截图记录未找到（找到 %d/%d 条记录）\n\n", validCount, screenshotCount))
		}
	}

	return sb.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getPeriodTypeName(periodType string) string {
	switch periodType {
	case "halfhour":
		return "半小时"
	case "hour":
		return "小时"
	case "day":
		return "日"
	case "week":
		return "周"
	case "month":
		return "月"
	case "year":
		return "年"
	default:
		return periodType
	}
}

// ImprovedReport contains the improved summary and analysis
type ImprovedReport struct {
	Summary          string
	Analysis         string
	ImprovementNotes string
}

// ImproveReport reads evaluation report and generates improved report based on feedback
func (e *Evaluator) ImproveReport(summary *storage.PeriodSummary, evaluationReportPath string, screenshotRecords map[string]*storage.ScreenshotRecord) (*ImprovedReport, error) {
	if summary == nil {
		return nil, fmt.Errorf("summary is nil")
	}

	if e.improvementPromptTemplate == nil {
		return nil, fmt.Errorf("improvement prompt template not configured")
	}

	// Read evaluation report
	evaluationContent, err := os.ReadFile(evaluationReportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read evaluation report: %w", err)
	}

	// Parse evaluation report to extract key information
	evaluationText := string(evaluationContent)

	// Build improvement prompt
	improvementPrompt, err := e.buildImprovementPrompt(summary, evaluationText, screenshotRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to build improvement prompt: %w", err)
	}

	// Call LLM to generate improved report
	req := analyzer.VisionRequest{
		Model:               e.analyzer.AnalysisModel,
		MaxCompletionTokens: e.analyzer.MaxCompletionTokens,
		Messages: []analyzer.Message{
			{
				Role: "user",
				Content: []analyzer.ContentObject{
					{
						Type: "text",
						Text: improvementPrompt,
					},
				},
			},
		},
	}

	improvedResult, err := e.callAPI(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call API for improvement: %w", err)
	}

	// Parse improved result
	improved := e.parseImprovedReport(improvedResult)
	return improved, nil
}

func (e *Evaluator) buildImprovementPrompt(summary *storage.PeriodSummary, evaluationText string, screenshotRecords map[string]*storage.ScreenshotRecord) (string, error) {
	if e.improvementPromptTemplate == nil {
		return "", fmt.Errorf("improvement prompt template not configured")
	}

	screenshotCount := len(strings.Split(summary.Screenshots, ","))
	if summary.Screenshots == "" {
		screenshotCount = 0
	}

	periodTypeName := getPeriodTypeName(summary.PeriodType)

	// Build screenshot source information using template
	// Filter out desktop/lock screen screenshots as they are not work activities
	screenshotSourceContent := e.buildScreenshotSourceContent(screenshotRecords, summary.Screenshots)

	// Format screenshot source using template if available, otherwise use default format
	var screenshotSource string
	if e.improvementScreenshotSourceTemplate != nil {
		var buf bytes.Buffer
		if err := e.improvementScreenshotSourceTemplate.Execute(&buf, map[string]interface{}{
			"ScreenshotSource": screenshotSourceContent,
		}); err != nil {
			return "", fmt.Errorf("failed to execute screenshot source template: %w", err)
		}
		screenshotSource = buf.String()
	} else {
		// Fallback if template not configured
		screenshotSource = screenshotSourceContent
	}

	// Prepare template data
	data := map[string]interface{}{
		"PeriodType":       periodTypeName,
		"StartTime":        summary.StartTime.Format("2006-01-02 15:04:05"),
		"EndTime":          summary.EndTime.Format("2006-01-02 15:04:05"),
		"ScreenshotCount":  screenshotCount,
		"EvaluationText":   evaluationText,
		"Summary":          summary.Summary,
		"Analysis":         summary.Analysis,
		"ScreenshotSource": screenshotSource,
	}

	// Execute template
	var buf bytes.Buffer
	if err := e.improvementPromptTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute improvement prompt template: %w", err)
	}

	return buf.String(), nil
}

// buildScreenshotSourceContent builds the screenshot source content with full analysis
// Returns formatted string with screenshot details, filtered to exclude desktop/lock screen
func (e *Evaluator) buildScreenshotSourceContent(screenshotRecords map[string]*storage.ScreenshotRecord, screenshotIDsStr string) string {
	if len(screenshotRecords) == 0 {
		return ""
	}

	var sb strings.Builder
	screenshotIDs := strings.Split(screenshotIDsStr, ",")
	count := 0
	skippedCount := 0

	for _, id := range screenshotIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if record, exists := screenshotRecords[id]; exists {
			// Filter out desktop/lock screen screenshots
			if record.Analysis == "" || isDesktopOrLockScreenAnalysis(record.Analysis) {
				skippedCount++
				continue
			}
			count++
			// Include FULL analysis content, not truncated
			// This ensures LLM has complete context to verify claims
			if count <= 20 { // Limit to first 20 screenshots to avoid prompt being too long
				sb.WriteString(fmt.Sprintf("### 截图 %s\n", id[:8]))
				sb.WriteString(fmt.Sprintf("- **完整ID**: %s\n", id))
				sb.WriteString(fmt.Sprintf("- **时间**: %s\n", record.Timestamp.Format("2006-01-02 15:04:05")))
				sb.WriteString(fmt.Sprintf("- **截图路径**: %s\n", record.ImagePath))
				sb.WriteString(fmt.Sprintf("- **完整分析内容**:\n%s\n\n", record.Analysis))
			}
		}
	}

	if skippedCount > 0 {
		sb.WriteString(fmt.Sprintf("**注意**: 已过滤 %d 张桌面/锁屏状态的截图（非工作活动）\n\n", skippedCount))
	}
	if count > 20 {
		sb.WriteString(fmt.Sprintf("**注意**: 还有 %d 张有效截图未列出，但评估和改进时请考虑所有截图\n\n", count-20))
	}
	if count == 0 {
		sb.WriteString("**重要**: 该时间段内所有截图均为桌面或锁屏状态，没有检测到有效工作活动。\n\n")
	}

	return sb.String()
}

func (e *Evaluator) parseImprovedReport(result string) *ImprovedReport {
	improved := &ImprovedReport{}

	// Parse the result to extract:
	// 【改进后的事实总结】...【改进后的改进建议】...【改进说明】...

	lines := strings.Split(result, "\n")
	var currentSection string
	var currentContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "【改进后的事实总结】") {
			if currentSection != "" {
				e.setSectionContent(improved, currentSection, currentContent.String())
			}
			currentSection = "summary"
			currentContent.Reset()
			continue
		}

		if strings.Contains(trimmed, "【改进后的改进建议】") {
			if currentSection != "" {
				e.setSectionContent(improved, currentSection, currentContent.String())
			}
			currentSection = "analysis"
			currentContent.Reset()
			continue
		}

		if strings.Contains(trimmed, "【改进说明】") {
			if currentSection != "" {
				e.setSectionContent(improved, currentSection, currentContent.String())
			}
			currentSection = "notes"
			currentContent.Reset()
			continue
		}

		if currentSection != "" {
			if currentContent.Len() > 0 {
				currentContent.WriteString("\n")
			}
			currentContent.WriteString(line)
		}
	}

	// Handle last section
	if currentSection != "" {
		e.setSectionContent(improved, currentSection, currentContent.String())
	}

	return improved
}

func (e *Evaluator) setSectionContent(improved *ImprovedReport, section, content string) {
	content = strings.TrimSpace(content)
	switch section {
	case "summary":
		improved.Summary = content
	case "analysis":
		improved.Analysis = content
	case "notes":
		improved.ImprovementNotes = content
	}
}

// isDesktopOrLockScreenAnalysis checks if the analysis content indicates desktop or lock screen state
// Returns true if the analysis suggests desktop/lock screen, false otherwise
// This function uses strict matching to avoid false positives with work-related screenshots
func isDesktopOrLockScreenAnalysis(analysis string) bool {
	if analysis == "" {
		return true // Empty analysis usually means desktop/lock screen was detected
	}

	// Convert to lowercase for case-insensitive matching
	lowerAnalysis := strings.ToLower(analysis)

	// Extract summary section for more precise matching
	// Only check the summary section to avoid false positives from detailed descriptions
	var summaryPart string
	if strings.Contains(lowerAnalysis, "【摘要】") {
		summaryStart := strings.Index(lowerAnalysis, "【摘要】")
		summaryEnd := strings.Index(lowerAnalysis, "【详细论述】")
		if summaryEnd > summaryStart {
			summaryPart = lowerAnalysis[summaryStart:summaryEnd]
		} else {
			// If no detailed section, use everything after summary marker
			summaryPart = lowerAnalysis[summaryStart:]
		}
	} else {
		// If no summary marker, use first 200 chars as summary
		if len(lowerAnalysis) > 200 {
			summaryPart = lowerAnalysis[:200]
		} else {
			summaryPart = lowerAnalysis
		}
	}

	// Strict patterns that indicate desktop or lock screen state
	// These patterns must appear in the summary section to avoid false positives
	// We use more specific patterns to avoid matching work-related activities
	strictPatterns := []string{
		"锁屏界面",
		"锁屏状态",
		"处于锁屏",
		"电脑桌面",
		"桌面界面",
		"桌面状态",
		"系统登录",
		"系统锁屏",
		"等待解锁",
		"等待输入密码",
		"没有打开任何应用程序",
		"没有打开任何应用",
		"lock screen",
		"lockscreen",
		"desktop",
	}

	// Check for strict patterns in summary
	for _, pattern := range strictPatterns {
		if strings.Contains(summaryPart, pattern) {
			return true
		}
	}

	// Special handling for "登录界面" - only match if it's system login, not application login
	// Pattern: "登录界面" but NOT "游戏.*登录界面" or "应用.*登录界面"
	if strings.Contains(summaryPart, "登录界面") {
		// Exclude game or application login screens
		if !strings.Contains(summaryPart, "游戏") &&
			!strings.Contains(summaryPart, "应用") &&
			!strings.Contains(summaryPart, "软件") {
			return true
		}
	}

	// Special handling for "输入密码" - only match if it's system unlock, not application login
	if strings.Contains(summaryPart, "输入密码") {
		// Check if it's about unlocking system (lock screen context)
		if strings.Contains(summaryPart, "解锁") ||
			strings.Contains(summaryPart, "锁屏") ||
			strings.Contains(summaryPart, "系统") {
			return true
		}
		// If it's about game/application login, don't filter
		if strings.Contains(summaryPart, "游戏") ||
			strings.Contains(summaryPart, "应用") {
			return false
		}
	}

	// Additional check: if summary is very short (< 100 chars) and contains lock/desktop keywords,
	// it's likely a non-work state
	if len(summaryPart) < 100 {
		if strings.Contains(summaryPart, "锁屏") || strings.Contains(summaryPart, "桌面") {
			return true
		}
	}

	return false
}
