package analyzer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type OpenAI struct {
	APIKey         string
	BaseURL        string // API base URL, supports OpenAI-compatible endpoints
	Model          string // Default model for screenshot analysis
	MaxCompletionTokens int
	Prompt         string // Prompt for screenshot analysis
	
	// Desktop/lock screen detection
	DesktopLockDetectionPrompt string // Prompt for desktop/lock screen detection
	LockScreenDetectionPrompt   string // Prompt for lock screen detection only
	
	// Summary configuration (frequent, simple task, cheaper model)
	SummaryModel   string
	SummaryPrompt  string
	SummaryEnhancedTemplate string // Enhanced summary prompt template
	SummaryContextPrefixTemplate string // Context prefix template
	SummaryRollingTemplate string // Rolling summary prompt template
	
	// Level-specific summary prompts
	FifteenminPrompt string
	HourPrompt       string
	DayPrompt        string
	WeekPrompt       string
	MonthPrompt      string
	QuarterPrompt    string
	YearPrompt       string
	
	// Analysis configuration (less frequent, complex task, stronger model)
	AnalysisModel  string
	AnalysisPrompt string
}

type VisionRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxCompletionTokens int `json:"max_completion_tokens"`
}

type Message struct {
	Role    string          `json:"role"`
	Content []ContentObject `json:"content"`
}

type ContentObject struct {
	Type     string  `json:"type"`
	Text     string  `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type VisionResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func NewOpenAI(apiKey, baseURL, model string, maxTokens int, prompt string, desktopLockDetectionPrompt string, lockScreenDetectionPrompt string, summaryModel, summaryPrompt, summaryEnhanced, summaryContextPrefix, summaryRolling, analysisModel, analysisPrompt string, levelPrompts ...map[string]string) *OpenAI {
	// Use default base URL if not provided
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	
	// Use default models if not provided
	if summaryModel == "" {
		summaryModel = "gpt-4o-mini"
	}
	if analysisModel == "" {
		analysisModel = "gpt-4o"
	}
	
	openAI := &OpenAI{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		Model:          model,
		MaxCompletionTokens: maxTokens,
		Prompt:         prompt,
		DesktopLockDetectionPrompt: desktopLockDetectionPrompt,
		LockScreenDetectionPrompt:   lockScreenDetectionPrompt,
		SummaryModel:   summaryModel,
		SummaryPrompt:  summaryPrompt,
		SummaryEnhancedTemplate: summaryEnhanced,
		SummaryContextPrefixTemplate: summaryContextPrefix,
		SummaryRollingTemplate: summaryRolling,
		AnalysisModel:  analysisModel,
		AnalysisPrompt: analysisPrompt,
	}
	
	// Set level-specific prompts if provided
	if len(levelPrompts) > 0 && levelPrompts[0] != nil {
		prompts := levelPrompts[0]
		if p, ok := prompts["fifteenmin"]; ok {
			openAI.FifteenminPrompt = p
		}
		if p, ok := prompts["hour"]; ok {
			openAI.HourPrompt = p
		}
		if p, ok := prompts["day"]; ok {
			openAI.DayPrompt = p
		}
		if p, ok := prompts["week"]; ok {
			openAI.WeekPrompt = p
		}
		if p, ok := prompts["month"]; ok {
			openAI.MonthPrompt = p
		}
		if p, ok := prompts["quarter"]; ok {
			openAI.QuarterPrompt = p
		}
		if p, ok := prompts["year"]; ok {
			openAI.YearPrompt = p
		}
	}
	
	return openAI
}

// IsLockScreen quickly checks if the screenshot is a lock screen
// Returns true if it's a lock screen, false otherwise
// Uses a simple prompt with cheaper model to minimize cost
func (o *OpenAI) IsLockScreen(imagePath string) (bool, error) {
	imageData, err := encodeImageToBase64(imagePath)
	if err != nil {
		return false, fmt.Errorf("failed to encode image: %w", err)
	}

	// Use configured prompt, return error if not configured
	detectionPrompt := o.LockScreenDetectionPrompt
	if detectionPrompt == "" {
		return false, fmt.Errorf("lock screen detection prompt not configured")
	}

	// Use cheaper model for quick detection
	model := o.SummaryModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	req := VisionRequest{
		Model:     model,
		MaxCompletionTokens: 50, // Allow brief explanation if needed
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentObject{
					{
						Type: "text",
						Text: detectionPrompt,
					},
					{
						Type: "image_url",
						ImageURL: &ImageURL{
							URL: fmt.Sprintf("data:image/png;base64,%s", imageData),
						},
					},
				},
			},
		},
	}

	content, err := o.callAPI(req)
	if err != nil {
		return false, fmt.Errorf("failed to detect lock screen: %w", err)
	}

	// Check if response indicates lock screen
	content = strings.ToLower(strings.TrimSpace(content))
	return strings.Contains(content, "是") || strings.Contains(content, "yes"), nil
}

// IsDesktopOrLockScreen quickly checks if the screenshot is desktop or lock screen
// Returns true if it's desktop or lock screen, false otherwise
// Uses a simple prompt with cheaper model to minimize cost
func (o *OpenAI) IsDesktopOrLockScreen(imagePath string) (bool, error) {
	// If detection prompt is not configured, skip detection and proceed with analysis
	if o.DesktopLockDetectionPrompt == "" {
		return false, nil
	}

	imageData, err := encodeImageToBase64(imagePath)
	if err != nil {
		return false, fmt.Errorf("failed to encode image: %w", err)
	}

	// Use configured prompt for detection
	detectionPrompt := o.DesktopLockDetectionPrompt

	// Use cheaper model for quick detection
	model := o.SummaryModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	req := VisionRequest{
		Model:     model,
		MaxCompletionTokens: 50, // Allow brief explanation if needed
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentObject{
					{
						Type: "text",
						Text: detectionPrompt,
					},
					{
						Type: "image_url",
						ImageURL: &ImageURL{
							URL: fmt.Sprintf("data:image/png;base64,%s", imageData),
						},
					},
				},
			},
		},
	}

	content, err := o.callAPI(req)
	if err != nil {
		return false, fmt.Errorf("failed to detect desktop/lock screen: %w", err)
	}

	// Check if response indicates desktop or lock screen
	content = strings.ToLower(strings.TrimSpace(content))
	return strings.Contains(content, "是") || strings.Contains(content, "yes"), nil
}

func (o *OpenAI) AnalyzeScreenshot(imagePath string) (string, error) {
	imageData, err := encodeImageToBase64(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to encode image: %w", err)
	}

	req := VisionRequest{
		Model:     o.Model,
		MaxCompletionTokens: o.MaxCompletionTokens,
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentObject{
					{
						Type: "text",
						Text: o.Prompt,
					},
					{
						Type: "image_url",
						ImageURL: &ImageURL{
							URL: fmt.Sprintf("data:image/png;base64,%s", imageData),
						},
					},
				},
			},
		},
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/chat/completions", o.BaseURL)
	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.APIKey))

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var visionResp VisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&visionResp); err != nil {
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

func encodeImageToBase64(imagePath string) (string, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		// Try to convert old flat path to new nested path if file not found
		if os.IsNotExist(err) {
			convertedPath := convertToNestedPath(imagePath)
			if convertedPath != imagePath {
				file, err = os.Open(convertedPath)
				if err == nil {
					defer file.Close()
					data, err := io.ReadAll(file)
					if err != nil {
						return "", err
					}
					return base64.StdEncoding.EncodeToString(data), nil
				}
			}
		}
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// convertToNestedPath converts old flat path format to new nested format with Q and W directories
// e.g., .../2025/12/09/14/18.png -> .../2025/Q4/12/W2/09/14/18.png
func convertToNestedPath(oldPath string) string {
	parts := strings.Split(filepath.ToSlash(oldPath), "/")
	
	// Find the year index
	yearIdx := -1
	for i, part := range parts {
		if len(part) == 4 && part >= "2000" && part <= "2099" {
			yearIdx = i
			break
		}
	}
	
	if yearIdx == -1 || yearIdx+3 >= len(parts) {
		return oldPath // Cannot convert
	}
	
	month := parts[yearIdx+1]
	day := parts[yearIdx+2]
	
	// Check if already in new format (has Q directory)
	if yearIdx+1 < len(parts) && strings.HasPrefix(parts[yearIdx+1], "Q") {
		return oldPath // Already in new format
	}
	
	// Calculate quarter: Q1-Q4
	monthInt, err := strconv.Atoi(month)
	if err != nil {
		return oldPath
	}
	quarter := (monthInt-1)/3 + 1
	quarterDir := fmt.Sprintf("Q%d", quarter)
	
	// Calculate Calendar Week: W1-W5 (month-based week number)
	dayInt, err := strconv.Atoi(day)
	if err != nil {
		return oldPath
	}
	weekNum := ((dayInt - 1) / 7) + 1
	weekDir := fmt.Sprintf("W%d", weekNum)
	
	// Build new path: insert Q and W directories
	// Preserve absolute path prefix if present
	isAbsolute := filepath.IsAbs(oldPath)
	newParts := make([]string, 0, len(parts)+2)
	
	if isAbsolute && parts[0] == "" {
		// Unix absolute path starts with empty string after split
		newParts = append(newParts, "")
		newParts = append(newParts, parts[1:yearIdx+1]...)   // up to and including year
	} else {
		newParts = append(newParts, parts[:yearIdx+1]...)     // up to and including year
	}
	
	newParts = append(newParts, quarterDir)               // add Q directory
	newParts = append(newParts, month)                    // add month
	newParts = append(newParts, weekDir)                  // add W directory
	newParts = append(newParts, parts[yearIdx+2:]...)     // add remaining (day, hour, file)
	
	result := filepath.Join(newParts...)
	// filepath.Join removes leading slash on Unix, restore it if original was absolute
	if isAbsolute && !filepath.IsAbs(result) {
		result = string(filepath.Separator) + result
	}
	return result
}

// GenerateSummary generates a concise summary of work activities during a period
// Uses cheaper model (summary_model) for frequent, simple tasks
// For longer periods (day, week, month), the summary should be more detailed
// periodType is optional - if provided, uses level-specific prompt if available
func (o *OpenAI) GenerateSummary(analysisText string, periodType ...string) (string, error) {
	return o.GenerateSummaryWithContext(analysisText, "", periodType...)
}

// GenerateSummaryWithContext generates a summary with progress context for logging
func (o *OpenAI) GenerateSummaryWithContext(analysisText string, progressContext string, periodType ...string) (string, error) {
	// Select prompt based on period type
	var selectedPrompt string
	if len(periodType) > 0 && periodType[0] != "" {
		switch periodType[0] {
		case "fifteenmin":
			if o.FifteenminPrompt != "" {
				selectedPrompt = o.FifteenminPrompt
			}
		case "hour":
			if o.HourPrompt != "" {
				selectedPrompt = o.HourPrompt
			}
		case "day":
			if o.DayPrompt != "" {
				selectedPrompt = o.DayPrompt
			}
		case "week":
			if o.WeekPrompt != "" {
				selectedPrompt = o.WeekPrompt
			}
		case "month":
			if o.MonthPrompt != "" {
				selectedPrompt = o.MonthPrompt
			}
		case "quarter":
			if o.QuarterPrompt != "" {
				selectedPrompt = o.QuarterPrompt
			}
		case "year":
			if o.YearPrompt != "" {
				selectedPrompt = o.YearPrompt
			}
		}
	}
	
	// Fallback to default prompt if no level-specific prompt found
	if selectedPrompt == "" {
		selectedPrompt = o.SummaryPrompt
	}
	
	// Combine summary prompt with the analysis text
	// Add instruction for longer periods to include more details
	enhancedPrompt := selectedPrompt
	// Estimate period length by counting newlines (each screenshot analysis is typically one line)
	lineCount := strings.Count(analysisText, "\n")
	if lineCount > 20 && o.SummaryEnhancedTemplate != "" {
		// For longer periods, request more detailed summary
		enhancedPrompt = strings.ReplaceAll(enhancedPrompt, "简洁", "详细且全面")
		enhancedPrompt += "\n\n" + o.SummaryEnhancedTemplate
	}
	fullPrompt := fmt.Sprintf("%s\n\n截图分析信息：\n%s", enhancedPrompt, analysisText)

	req := VisionRequest{
		Model:     o.SummaryModel,
		MaxCompletionTokens: o.MaxCompletionTokens,
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentObject{
					{
						Type: "text",
						Text: fullPrompt,
					},
				},
			},
		},
	}
	
	return o.callAPIWithContext(req, progressContext)
}

// GenerateRollingSummary generates a rolling summary that combines previous summary with new content
// This implements progressive summarization: previous summary + new content -> compressed summary
// Similar items are merged and compressed to avoid redundancy
func (o *OpenAI) GenerateRollingSummary(previousSummary string, newContent string) (string, error) {
	return o.GenerateRollingSummaryWithContext(previousSummary, newContent, "")
}

// GenerateRollingSummaryWithContext generates a rolling summary with progress context for logging
func (o *OpenAI) GenerateRollingSummaryWithContext(previousSummary string, newContent string, progressContext string) (string, error) {
	// If rolling template is not configured, fallback to regular summary
	if o.SummaryRollingTemplate == "" {
		combinedText := previousSummary
		if newContent != "" {
			combinedText += "\n\n" + newContent
		}
		return o.GenerateSummaryWithContext(combinedText, progressContext)
	}

	// Build rolling summary prompt
	var inputText strings.Builder
	inputText.WriteString(o.SummaryRollingTemplate)
	inputText.WriteString("\n\n")
	
	if previousSummary != "" {
		inputText.WriteString("=== 前序汇总 ===\n\n")
		inputText.WriteString(previousSummary)
		inputText.WriteString("\n\n")
	}
	
	if newContent != "" {
		inputText.WriteString("=== 新增内容 ===\n\n")
		inputText.WriteString(newContent)
	}

	fullPrompt := inputText.String()

	req := VisionRequest{
		Model:     o.SummaryModel,
		MaxCompletionTokens: o.MaxCompletionTokens,
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentObject{
					{
						Type: "text",
						Text: fullPrompt,
					},
				},
			},
		},
	}
	
	return o.callAPIWithContext(req, progressContext)
}

// AnalyzeBehavior performs deep behavior analysis and provides efficiency improvement suggestions
// Uses stronger model (analysis_model) for less frequent, complex tasks
func (o *OpenAI) AnalyzeBehavior(summaryText string) (string, error) {
	// Combine analysis prompt with the summary text
	fullPrompt := fmt.Sprintf("%s\n\n工作活动摘要：\n%s", o.AnalysisPrompt, summaryText)

	req := VisionRequest{
		Model:     o.AnalysisModel,
		MaxCompletionTokens: o.MaxCompletionTokens,
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentObject{
					{
						Type: "text",
						Text: fullPrompt,
					},
				},
			},
		},
	}
	
	return o.callAPI(req)
}

// callAPI is a helper method to make API calls with adaptive retry logic
func (o *OpenAI) callAPI(req VisionRequest) (string, error) {
	return o.callAPIWithContext(req, "")
}

// callAPIWithContext calls the API with optional progress context for logging
func (o *OpenAI) callAPIWithContext(req VisionRequest, progressContext string) (string, error) {
	const maxRetries = 5 // 增加重试次数
	const initialBackoff = 2 * time.Second
	
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 自适应退避策略
			backoff := calculateBackoff(attempt, initialBackoff, lastErr)
			fmt.Fprintf(os.Stderr, "time=\"%s\" level=info msg=\"Retrying API request (attempt %d/%d, backoff: %v, reason: %s)\"\n",
				time.Now().Format("2006-01-02 15:04:05"), attempt+1, maxRetries+1, backoff, getErrorType(lastErr))
			time.Sleep(backoff)
		}
		
		result, err := o.callAPISingleWithContext(req, attempt == 0, progressContext)
		if err == nil {
			// 成功时记录，帮助调试
			if attempt > 0 {
				fmt.Fprintf(os.Stderr, "time=\"%s\" level=info msg=\"API request succeeded after %d retries\"\n",
					time.Now().Format("2006-01-02 15:04:05"), attempt)
			}
			return result, nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return "", err
		}
		
		// 对于最后一次重试前，增加额外的等待时间
		if attempt == maxRetries-1 {
			fmt.Fprintf(os.Stderr, "time=\"%s\" level=warning msg=\"Last retry attempt, adding extra backoff time\"\n",
				time.Now().Format("2006-01-02 15:04:05"))
		}
	}
	
	return "", fmt.Errorf("API call failed after %d retries: %w", maxRetries+1, lastErr)
}

// calculateBackoff 计算自适应退避时间
func calculateBackoff(attempt int, initialBackoff time.Duration, lastErr error) time.Duration {
	// 基础指数退避
	baseBackoff := initialBackoff * time.Duration(1<<uint(attempt-1))
	
	if lastErr == nil {
		return baseBackoff
	}
	
	errStr := lastErr.Error()
	
	// 对于限流错误 (429)，使用更长的退避时间
	if strings.Contains(errStr, "status 429") || strings.Contains(errStr, "rate limit") {
		return baseBackoff * 3 // 限流时等待 3 倍时间
	}
	
	// 对于超时错误，使用较长的退避时间
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "i/o timeout") {
		return baseBackoff * 2 // 超时时等待 2 倍时间
	}
	
	// 对于网络连接错误，使用较长的退避时间
	if strings.Contains(errStr, "dial tcp") || strings.Contains(errStr, "connection refused") {
		return baseBackoff * 2
	}
	
	// 其他错误使用基础退避
	return baseBackoff
}

// getErrorType 获取错误类型的简短描述
func getErrorType(err error) string {
	if err == nil {
		return "unknown"
	}
	
	errStr := err.Error()
	
	if strings.Contains(errStr, "status 429") || strings.Contains(errStr, "rate limit") {
		return "rate_limit"
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "i/o timeout") {
		return "timeout"
	}
	if strings.Contains(errStr, "dial tcp") {
		return "connection_failed"
	}
	if strings.Contains(errStr, "status 502") {
		return "bad_gateway"
	}
	if strings.Contains(errStr, "status 503") {
		return "service_unavailable"
	}
	if strings.Contains(errStr, "status 504") {
		return "gateway_timeout"
	}
	if strings.Contains(errStr, "status 500") {
		return "internal_server_error"
	}
	
	return "other_error"
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
func (o *OpenAI) callAPISingle(req VisionRequest, logProgress bool) (string, error) {
	return o.callAPISingleWithContext(req, logProgress, "")
}

// callAPISingleWithContext makes a single API call with optional progress context
func (o *OpenAI) callAPISingleWithContext(req VisionRequest, logProgress bool, progressContext string) (string, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	endpoint := fmt.Sprintf("%s/chat/completions", o.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.APIKey))

	// Start progress logging in a goroutine
	progressDone := make(chan bool)
	if logProgress {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			startTime := time.Now()
			for {
				select {
				case <-ticker.C:
					elapsed := time.Since(startTime)
					if progressContext != "" {
						fmt.Fprintf(os.Stderr, "time=\"%s\" level=info msg=\"API request in progress (elapsed: %v, %s)\"\n",
							time.Now().Format("2006-01-02 15:04:05"), elapsed.Round(time.Second), progressContext)
					} else {
						fmt.Fprintf(os.Stderr, "time=\"%s\" level=info msg=\"API request in progress (elapsed: %v)\"\n",
							time.Now().Format("2006-01-02 15:04:05"), elapsed.Round(time.Second))
					}
				case <-progressDone:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	resp, err := client.Do(httpReq)
	if logProgress {
		close(progressDone)
	}
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

	var visionResp VisionResponse
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

