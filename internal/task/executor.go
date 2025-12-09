package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"stuff-time/internal/analyzer"
	"stuff-time/internal/config"
	"stuff-time/internal/logger"
	"stuff-time/internal/screenshot"
	"stuff-time/internal/storage"
)

type Executor struct {
	config         *config.Config
	storage        *storage.Storage
	storageManager *storage.StorageManager
	analyzer       *analyzer.OpenAI
	analysisMutex  sync.Mutex
	isAnalyzing    bool
}

func NewExecutor(cfg *config.Config, st *storage.Storage) (*Executor, error) {
	if cfg.OpenAI.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	// 创建 StorageManager
	storageManager := storage.NewStorageManager(&cfg.Storage, cfg.Storage.ReportsPath)

	// Build level-specific prompts map
	levelPrompts := make(map[string]string)
	if cfg.OpenAI.FifteenminPromptContent != "" {
		levelPrompts["fifteenmin"] = cfg.OpenAI.FifteenminPromptContent
	}
	if cfg.OpenAI.HourPromptContent != "" {
		levelPrompts["hour"] = cfg.OpenAI.HourPromptContent
	}
	if cfg.OpenAI.DayPromptContent != "" {
		levelPrompts["day"] = cfg.OpenAI.DayPromptContent
	}
	if cfg.OpenAI.WeekPromptContent != "" {
		levelPrompts["week"] = cfg.OpenAI.WeekPromptContent
	}
	if cfg.OpenAI.MonthPromptContent != "" {
		levelPrompts["month"] = cfg.OpenAI.MonthPromptContent
	}
	if cfg.OpenAI.QuarterPromptContent != "" {
		levelPrompts["quarter"] = cfg.OpenAI.QuarterPromptContent
	}
	if cfg.OpenAI.YearPromptContent != "" {
		levelPrompts["year"] = cfg.OpenAI.YearPromptContent
	}

	analyzer := analyzer.NewOpenAI(
		cfg.OpenAI.APIKey,
		cfg.OpenAI.BaseURL,
		cfg.OpenAI.Model,
		cfg.OpenAI.MaxCompletionTokens,
		cfg.OpenAI.PromptContent,
		cfg.OpenAI.DesktopLockDetectionPromptContent,
		cfg.OpenAI.LockScreenDetectionPromptContent,
		cfg.OpenAI.SummaryModel,
		cfg.OpenAI.SummaryPromptContent,
		cfg.OpenAI.SummaryEnhancedContent,
		cfg.OpenAI.SummaryContextPrefixContent,
		cfg.OpenAI.SummaryRollingContent,
		cfg.OpenAI.AnalysisModel,
		cfg.OpenAI.AnalysisPromptContent,
		levelPrompts,
	)

	return &Executor{
		config:         cfg,
		storage:        st,
		storageManager: storageManager,
		analyzer:       analyzer,
	}, nil
}

func (e *Executor) CaptureScreenshot() error {
	logger.GetLogger().Info("Starting screenshot capture...")

	// Check if screen is locked
	logger.GetLogger().Debug("Checking screen lock status...")
	locked, err := screenshot.IsScreenLocked()
	if err != nil {
		logger.GetLogger().Warnf("Failed to check screen lock status: %v, proceeding anyway", err)
	} else if locked {
		logger.GetLogger().Info("Screen is locked, skipping screenshot capture")
		return nil // Skip screenshot when locked
	} else {
		logger.GetLogger().Debug("Screen is not locked, proceeding with screenshot capture")
	}

	// Check if current time is within work hours
	now := time.Now()
	if !e.config.Screenshot.WorkHours.IsWorkTime(now) {
		logger.GetLogger().Info("Outside work hours, skipping screenshot capture")
		return nil // Skip screenshot when outside work hours
	}

	screenID, err := screenshot.GetMouseScreenID()
	if err != nil {
		return fmt.Errorf("failed to get mouse screen ID: %w", err)
	}
	logger.GetLogger().Infof("Mouse screen ID: %d", screenID)

	logger.GetLogger().Infof("Capturing screen %d...", screenID)
	imagePath, err := screenshot.CaptureScreen(
		screenID,
		e.config.Screenshot.StoragePath,
		e.config.Screenshot.ImageFormat,
	)
	if err != nil {
		return fmt.Errorf("failed to capture screen: %w", err)
	}
	logger.GetLogger().Infof("Screen captured, saving to: %s", imagePath)

	record := storage.NewScreenshotRecord(screenID, imagePath)

	logger.GetLogger().Info("Saving screenshot record to database...")
	if err := e.storage.SaveScreenshot(record); err != nil {
		return fmt.Errorf("failed to save screenshot record: %w", err)
	}

	logger.GetLogger().Infof("Screenshot captured: %s (screen %d, path: %s)",
		record.ID, screenID, imagePath)

	return nil
}

// BatchAnalyze triggers batch analysis asynchronously to avoid blocking the scheduler
// If analysis is already in progress, it will skip this trigger
func (e *Executor) BatchAnalyze() error {
	// Try to acquire lock, if already analyzing, skip this trigger
	if !e.analysisMutex.TryLock() {
		logger.GetLogger().Info("Analysis already in progress, skipping this trigger")
		return nil
	}

	// Start analysis in a separate goroutine to avoid blocking the scheduler
	go func() {
		defer e.analysisMutex.Unlock()
		e.isAnalyzing = true
		defer func() { e.isAnalyzing = false }()

		if err := e.doBatchAnalyze(); err != nil {
			logger.GetLogger().Infof("ERROR: Batch analysis failed: %v",
				err)
		}
	}()

	return nil
}

// doBatchAnalyze performs the actual batch analysis work using worker pool for concurrency
func (e *Executor) doBatchAnalyze() error {
	records, err := e.storage.GetUnanalyzedScreenshots(100)
	if err != nil {
		return fmt.Errorf("failed to get unanalyzed screenshots: %w", err)
	}

	if len(records) == 0 {
		logger.GetLogger().Info("No unanalyzed screenshots found")
		// Even if no unanalyzed screenshots, check for outdated reports in recent hours
		// Regenerate reports for the current hour
		now := time.Now()
		currentHourKey := now.Format("2006-01-02-15")
		e.regenerateReportsForAnalyzedScreenshots(currentHourKey)
		return nil
	}

	// Also regenerate reports for analyzed screenshots that might have outdated reports
	// This fixes reports that were generated before analysis completed
	// Process the hour of the first unanalyzed screenshot
	e.regenerateReportsForAnalyzedScreenshots(records[0].HourKey)

	// Determine worker count
	workerCount := e.config.Screenshot.AnalysisWorkers
	if workerCount <= 0 {
		workerCount = 3 // Default to 3 workers
	}
	if workerCount > len(records) {
		workerCount = len(records) // Don't create more workers than jobs
	}

	logger.GetLogger().Infof("Starting batch analysis for %d screenshots with %d workers",
		len(records), workerCount)

	// Use worker pool for concurrent analysis
	return e.doBatchAnalyzeWithWorkers(records, workerCount)
}

// analysisResult represents the result of analyzing a single screenshot
type analysisResult struct {
	record   *storage.ScreenshotRecord
	analysis string
	err      error
}

// doBatchAnalyzeWithWorkers performs batch analysis using worker pool pattern
func (e *Executor) doBatchAnalyzeWithWorkers(records []*storage.ScreenshotRecord, workerCount int) error {
	// Create channels for jobs and results
	jobs := make(chan *storage.ScreenshotRecord, len(records))
	results := make(chan analysisResult, len(records))

	// Start workers
	for w := 0; w < workerCount; w++ {
		go e.analysisWorker(w, jobs, results)
	}

	// Send jobs
	for _, record := range records {
		jobs <- record
	}
	close(jobs)

	// Collect results
	successCount := 0
	failCount := 0

	for i := 0; i < len(records); i++ {
		result := <-results
		record := result.record

		// Skip desktop or lock screen screenshots (empty analysis means skip)
		if result.analysis == "" && result.err == nil {
			logger.GetLogger().Infof("Skipping desktop/lock screen screenshot %s (no analysis needed)",
				record.ID)
			// Mark as analyzed but with empty analysis to indicate it was skipped
			if err := e.storage.UpdateScreenshotAnalysis(record.ID, ""); err != nil {
				logger.GetLogger().Infof("ERROR: Failed to mark screenshot %s as skipped: %v",
					record.ID, err)
			}
			continue
		}

		if result.err != nil {
			logger.GetLogger().Infof("WARNING: Failed to analyze screenshot %s: %v",
				record.ID, result.err)
			result.analysis = fmt.Sprintf("Analysis failed: %v", result.err)
			failCount++
		} else {
			successCount++
		}

		// Update record.Analysis BEFORE saving to database, so saveReport can use it
		record.Analysis = result.analysis

		if err := e.storage.UpdateScreenshotAnalysis(record.ID, result.analysis); err != nil {
			logger.GetLogger().Infof("ERROR: Failed to update analysis for %s: %v",
				record.ID, err)
			failCount++
		} else {
			logger.GetLogger().Infof("Analysis completed for screenshot: %s",
				record.ID)
		}

		if err := e.updateHourSummary(record); err != nil {
			logger.GetLogger().Infof("ERROR: Failed to update hour summary for %s: %v",
				record.HourKey, err)
		}

		// Save report to file (always save, even if database update failed)
		// This ensures report reflects the analysis result
		if err := e.saveReport(record); err != nil {
			logger.GetLogger().Infof("WARNING: Failed to save report for %s: %v",
				record.ID, err)
		}
	}

	logger.GetLogger().Infof("Batch analysis completed: %d succeeded, %d failed",
		successCount, failCount)

	return nil
}

// analysisWorker is a worker that processes analysis jobs from the jobs channel
func (e *Executor) analysisWorker(workerID int, jobs <-chan *storage.ScreenshotRecord, results chan<- analysisResult) {
	for record := range jobs {
		// First check if it's desktop or lock screen, skip analysis if so
		isDesktopOrLockScreen, err := e.analyzer.IsDesktopOrLockScreen(record.ImagePath)
		if err != nil {
			logger.GetLogger().Infof("WARNING: Failed to detect desktop/lock screen for %s: %v, proceeding with analysis",
				record.ID, err)
			// Continue with analysis if detection fails
		} else if isDesktopOrLockScreen {
			// Skip analysis for desktop or lock screen
			logger.GetLogger().Infof("Skipping analysis for %s: detected desktop or lock screen", record.ID)
			results <- analysisResult{
				record:   record,
				analysis: "", // Empty analysis means skip
				err:      nil,
			}
			continue
		}

		// Proceed with normal analysis
		analysis, err := e.analyzer.AnalyzeScreenshot(record.ImagePath)
		results <- analysisResult{
			record:   record,
			analysis: analysis,
			err:      err,
		}
	}
}

func (e *Executor) GeneratePeriodSummary(forceFromScreenshots bool, isManual bool) error {
	summaryPeriods := e.config.Screenshot.SummaryPeriods
	if len(summaryPeriods) == 0 {
		summaryPeriods = []string{"hour", "day", "week", "month"}
	}

	now := time.Now()
	var errors []string

	for _, periodType := range summaryPeriods {
		if err := e.generateSinglePeriodSummary(now, periodType, forceFromScreenshots, isManual); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", periodType, err))
			logger.GetLogger().Infof("WARNING: Failed to generate %s summary: %v",
				periodType, err)
		}
	}

	if len(errors) > 0 && len(errors) == len(summaryPeriods) {
		return fmt.Errorf("failed to generate all period summaries: %v", errors)
	}

	return nil
}

// GenerateSinglePeriodSummary generates a summary for a specific period type and optional date
func (e *Executor) GenerateSinglePeriodSummary(periodType string, dateStr string, forceFromScreenshots bool) error {
	var now time.Time
	if dateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return fmt.Errorf("invalid date format: %w", err)
		}
		now = parsedDate
	} else {
		now = time.Now()
	}

	// Manual generation always allows generating current period
	return e.generateSinglePeriodSummary(now, periodType, forceFromScreenshots, true)
}

// GenerateHigherLevelSummaries generates all higher-level summaries from a given period type and date
// This allows starting from any level and aggregating upward
// All intermediate level reports will be updated
func (e *Executor) GenerateHigherLevelSummaries(periodType string, dateStr string, forceFromScreenshots bool) error {
	var periodTime time.Time
	if dateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return fmt.Errorf("invalid date format: %w", err)
		}
		periodTime = parsedDate
	} else {
		periodTime = time.Now()
	}

	// Adjust periodTime based on period type to get the correct period start time
	switch periodType {
	case "work-segment":
		// Use current day
		periodTime = time.Date(periodTime.Year(), periodTime.Month(), periodTime.Day(), 0, 0, 0, 0, periodTime.Location())
	case "day":
		periodTime = time.Date(periodTime.Year(), periodTime.Month(), periodTime.Day(), 0, 0, 0, 0, periodTime.Location())
	case "week":
		weekday := int(periodTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		periodTime = time.Date(periodTime.Year(), periodTime.Month(), periodTime.Day(), 0, 0, 0, 0, periodTime.Location())
		periodTime = periodTime.AddDate(0, 0, -(weekday - 1))
	case "month":
		periodTime = time.Date(periodTime.Year(), periodTime.Month(), 1, 0, 0, 0, 0, periodTime.Location())
	case "year":
		periodTime = time.Date(periodTime.Year(), 1, 1, 0, 0, 0, 0, periodTime.Location())
	}

	// GenerateHigherLevelSummaries is always called manually, so pass true
	return e.generateHigherLevelSummaries(periodType, periodTime, forceFromScreenshots, true)
}

func (e *Executor) generateSinglePeriodSummary(now time.Time, periodType string, forceFromScreenshots bool, isManual bool) error {
	var startTime, endTime time.Time
	var periodKey string

	switch periodType {
	case "fifteenmin":
		minute := now.Minute()
		// Round down to nearest 15-minute boundary (0, 15, 30, 45)
		roundedMinute := (minute / 15) * 15
		startTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), roundedMinute, 0, 0, now.Location())
		endTime = startTime.Add(15 * time.Minute)
		periodKey = startTime.Format("2006-01-02-15-04")
	case "hour":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		endTime = startTime.Add(time.Hour)
		periodKey = startTime.Format("2006-01-02-15")
	case "work-segment":
		// Work-segment is handled by generateWorkSegmentSummary
		// This case should not be reached in normal flow
		return fmt.Errorf("work-segment should be generated via generateWorkSegmentSummary")
	case "day":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endTime = startTime.AddDate(0, 0, 1)
		periodKey = startTime.Format("2006-01-02")
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		startTime = startTime.AddDate(0, 0, -(weekday - 1))
		endTime = startTime.AddDate(0, 0, 7)
		periodKey = startTime.Format("2006-01-02") + "-week"
	case "month":
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endTime = startTime.AddDate(0, 1, 0)
		periodKey = startTime.Format("2006-01")
	case "quarter":
		// Quarter: Q1 (Jan-Mar), Q2 (Apr-Jun), Q3 (Jul-Sep), Q4 (Oct-Dec)
		quarter := (int(now.Month())-1)/3 + 1
		quarterStartMonth := (quarter-1)*3 + 1
		startTime = time.Date(now.Year(), time.Month(quarterStartMonth), 1, 0, 0, 0, 0, now.Location())
		endTime = startTime.AddDate(0, 3, 0)
		periodKey = fmt.Sprintf("%d-Q%d", now.Year(), quarter)
	case "year":
		startTime = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		endTime = startTime.AddDate(1, 0, 0)
		periodKey = startTime.Format("2006")
	default:
		return fmt.Errorf("unsupported summary period: %s", periodType)
	}

	// For automatic generation, skip periods that haven't ended yet
	// Manual generation always allows generating current period
	if !isManual {
		currentTime := time.Now()
		// Check if the period has ended
		// For week, month, quarter, year: period must have ended
		// For shorter periods (fifteenmin, hour, day): always allow (they're based on current time)
		switch periodType {
		case "week", "month", "quarter", "year":
			if currentTime.Before(endTime) {
				logger.GetLogger().Infof("Skipping %s summary generation for %s: period not ended yet (ends at %s)",
					periodType, periodKey, endTime.Format(time.RFC3339))
				return nil
			}
		}
	}

	// Note: generate command always regenerates the current level summary, even if it exists.
	// forceFromScreenshots only affects how lower-level summaries are generated:
	// - false: use existing lower-level summaries, only generate missing ones
	// - true: force rebuild all lower-level summaries from screenshots layer by layer

	// Query actual data to determine the real time range
	// If no data exists in the theoretical range, return early (no report needed)
	actualStartTime, actualEndTime, hasData := e.determineActualTimeRange(periodType, startTime, endTime)
	if !hasData {
		logger.GetLogger().Infof("No data found for %s (%s to %s), skipping report generation",
			periodKey, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
		return nil
	}

	// Update time range based on actual data
	startTime = actualStartTime
	endTime = actualEndTime

	var periodSummary string
	var improvementAnalysis string
	var allScreenshotIDs []string
	screenshotIDSet := make(map[string]bool) // Use map for deduplication

	// Determine if we should aggregate from lower-level summaries or from screenshots
	lowerLevelType := e.getLowerLevelPeriodType(periodType)

	if lowerLevelType != "" {
		// Aggregate from lower-level summaries
		logger.GetLogger().Infof("DEBUG: Querying %s summaries from %s to %s", lowerLevelType, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
		lowerSummaries, err := e.storage.QueryPeriodSummaries(lowerLevelType, startTime, endTime)
		if err != nil {
			return fmt.Errorf("failed to query lower-level summaries: %w", err)
		}
		logger.GetLogger().Infof("DEBUG: Found %d %s summaries", len(lowerSummaries), lowerLevelType)

		// If forceFromScreenshots=true, force rebuild all lower-level summaries from screenshots
		// Otherwise, only generate if missing
		if forceFromScreenshots {
			logger.GetLogger().Infof("Force rebuild: regenerating all %s summaries from screenshots for %s",
				lowerLevelType, periodKey)
			// Force rebuild all lower-level summaries from screenshots layer by layer
			if err := e.generateLowerLevelSummaries(lowerLevelType, startTime, endTime, forceFromScreenshots, isManual); err != nil {
				logger.GetLogger().Infof("WARNING: Failed to generate lower-level summaries: %v",
					err)
				// Continue anyway, will try to aggregate from screenshots as fallback
			}

			// Query again after generation
			lowerSummaries, err = e.storage.QueryPeriodSummaries(lowerLevelType, startTime, endTime)
			if err != nil {
				return fmt.Errorf("failed to query lower-level summaries after generation: %w", err)
			}

			// If still no summaries, fallback to screenshots
			if len(lowerSummaries) == 0 {
				logger.GetLogger().Infof("Still no %s summaries found, falling back to screenshots for %s",
					lowerLevelType, periodKey)
				lowerLevelType = "" // Fallback to screenshot aggregation
			}
		} else if len(lowerSummaries) == 0 {
			// If no lower-level summaries found, check if there's any screenshot data first
			// This avoids unnecessary generation attempts when there's no data at all
			screenshots, err := e.storage.QueryByDateRange(startTime, endTime)
			if err != nil {
				logger.GetLogger().Infof("WARNING: Failed to query screenshots for %s: %v", periodKey, err)
			}

			// Filter out desktop/lock screen screenshots to check if there's any valid work activity
			var validScreenshots []*storage.ScreenshotRecord
			if err == nil {
				for _, s := range screenshots {
					if s.Analysis != "" && !strings.HasPrefix(s.Analysis, "Analysis failed") {
						if !isDesktopOrLockScreenAnalysis(s.Analysis) {
							validScreenshots = append(validScreenshots, s)
						}
					}
				}
			}

			// If no valid screenshots, skip generation and fallback to screenshot aggregation
			// This will eventually return nil when no valid content is found
			if len(validScreenshots) == 0 {
				logger.GetLogger().Infof("No valid screenshots found for %s, skipping lower-level generation, will fallback to screenshots",
					periodKey)
				lowerLevelType = "" // Fallback to screenshot aggregation
			} else {
				// There are valid screenshots, try to generate lower-level summaries
				logger.GetLogger().Infof("No %s summaries found for %s, generating them first...",
					lowerLevelType, periodKey)

				// Generate all lower-level summaries for this period
				// forceFromScreenshots=false: only generate missing lower-level summaries, use existing ones
				if err := e.generateLowerLevelSummaries(lowerLevelType, startTime, endTime, forceFromScreenshots, isManual); err != nil {
					logger.GetLogger().Infof("WARNING: Failed to generate lower-level summaries: %v",
						err)
					// Continue anyway, will try to aggregate from screenshots as fallback
				}

				// Query again after generation
				lowerSummaries, err = e.storage.QueryPeriodSummaries(lowerLevelType, startTime, endTime)
				if err != nil {
					return fmt.Errorf("failed to query lower-level summaries after generation: %w", err)
				}

				// If still no summaries, fallback to screenshots
				if len(lowerSummaries) == 0 {
					logger.GetLogger().Infof("Still no %s summaries found, falling back to screenshots for %s",
						lowerLevelType, periodKey)
					lowerLevelType = "" // Fallback to screenshot aggregation
				}
			}
		}

		var summaryTexts []string
		var invalidSummaryKeys []string
		var validLowerSummaries []*storage.PeriodSummary

		for _, s := range lowerSummaries {
			// Check if summary is a placeholder (already checked, no work activity)
			// Placeholders should be skipped, not regenerated
			if s.Summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
				logger.GetLogger().Infof("Placeholder summary detected for %s (%s), skipping (already checked, no work activity)",
					s.PeriodKey, lowerLevelType)
				// Don't include placeholder summaries in aggregation
				// Don't collect screenshot IDs from placeholder summaries
				continue
			}

			// Check if summary is invalid (contains "no work activity" message) or empty
			// Only collect screenshot IDs from valid summaries with actual work activity
			if s.Summary == "" || isInvalidSummary(s.Summary) {
				logger.GetLogger().Infof("Invalid or empty summary detected for %s (%s), will regenerate from lower level",
					s.PeriodKey, lowerLevelType)
				invalidSummaryKeys = append(invalidSummaryKeys, s.PeriodKey)
				// Don't include invalid/empty summaries in aggregation
				// Don't collect screenshot IDs from invalid summaries
				continue
			}

			// Only collect screenshot IDs from valid summaries
			if s.Screenshots != "" {
				ids := strings.Split(s.Screenshots, ",")
				for _, id := range ids {
					id = strings.TrimSpace(id)
					if id != "" {
						screenshotIDSet[id] = true
					}
				}
			}

			// Add valid summary to aggregation
			summaryTexts = append(summaryTexts, s.Summary)
			validLowerSummaries = append(validLowerSummaries, s)
		}

		// Regenerate invalid summaries from lower level
		if len(invalidSummaryKeys) > 0 {
			logger.GetLogger().Infof("Regenerating %d invalid %s summaries from lower level for %s",
				len(invalidSummaryKeys), lowerLevelType, periodKey)

			// Get the lower level type for regeneration
			lowerLowerLevelType := e.getLowerLevelPeriodType(lowerLevelType)
			if lowerLowerLevelType != "" {
				// Regenerate each invalid summary from its lower level
				for _, invalidKey := range invalidSummaryKeys {
					// Find the invalid summary to get its time range
					var invalidSummary *storage.PeriodSummary
					for _, s := range lowerSummaries {
						if s.PeriodKey == invalidKey {
							invalidSummary = s
							break
						}
					}

					if invalidSummary != nil {
						// Regenerate from lower level
						// Use forceFromScreenshots parameter to determine if we should rebuild from screenshots
						logger.GetLogger().Infof("Regenerating %s (%s) from %s level (forceFromScreenshots=%v)",
							invalidKey, lowerLevelType, lowerLowerLevelType, forceFromScreenshots)
						if err := e.generateSinglePeriodSummary(invalidSummary.StartTime, lowerLevelType, forceFromScreenshots, isManual); err != nil {
							logger.GetLogger().Infof("WARNING: Failed to regenerate invalid summary %s: %v",
								invalidKey, err)
						} else {
							// Query the regenerated summary
							regenerated, err := e.storage.GetPeriodSummary(invalidKey)
							if err == nil && regenerated != nil && regenerated.Summary != "" && !isInvalidSummary(regenerated.Summary) {
								// Use the regenerated summary
								summaryTexts = append(summaryTexts, regenerated.Summary)
								validLowerSummaries = append(validLowerSummaries, regenerated)
								// Add screenshot IDs to deduplication set
								if regenerated.Screenshots != "" {
									ids := strings.Split(regenerated.Screenshots, ",")
									for _, id := range ids {
										id = strings.TrimSpace(id)
										if id != "" {
											screenshotIDSet[id] = true
										}
									}
								}
								logger.GetLogger().Infof("Successfully regenerated %s from lower level", invalidKey)
							}
						}
					}
				}
			} else {
				// If no lower level available, regenerate from screenshots
				logger.GetLogger().Infof("No lower level available for %s, regenerating from screenshots", lowerLevelType)
				for _, invalidKey := range invalidSummaryKeys {
					var invalidSummary *storage.PeriodSummary
					for _, s := range lowerSummaries {
						if s.PeriodKey == invalidKey {
							invalidSummary = s
							break
						}
					}

					if invalidSummary != nil {
						// Regenerate from screenshots (no lower level available)
						if err := e.generateSinglePeriodSummary(invalidSummary.StartTime, lowerLevelType, true, isManual); err != nil {
							logger.GetLogger().Infof("WARNING: Failed to regenerate invalid summary %s from screenshots: %v",
								invalidKey, err)
						} else {
							// Query the regenerated summary
							regenerated, err := e.storage.GetPeriodSummary(invalidKey)
							if err == nil && regenerated != nil && regenerated.Summary != "" && !isInvalidSummary(regenerated.Summary) {
								summaryTexts = append(summaryTexts, regenerated.Summary)
								validLowerSummaries = append(validLowerSummaries, regenerated)
								// Add screenshot IDs to deduplication set
								if regenerated.Screenshots != "" {
									ids := strings.Split(regenerated.Screenshots, ",")
									for _, id := range ids {
										id = strings.TrimSpace(id)
										if id != "" {
											screenshotIDSet[id] = true
										}
									}
								}
								logger.GetLogger().Infof("Successfully regenerated %s from screenshots", invalidKey)
							}
						}
					}
				}
			}
		}

		if len(summaryTexts) > 0 {
			// Determine if we should use direct merge or LLM processing
			// For natural period summaries from already-aggregated levels (work-segment, day, etc.),
			// directly merge the summaries without LLM processing
			shouldDirectMerge := false
			if lowerLevelType != "" {
				// Check if lower level is already an aggregated level (not raw screenshots)
				aggregatedLevels := map[string]bool{
					"work-segment": true,
					"day":          true,
					"week":         true,
					"month":        true,
					"quarter":      true,
					"year":         true,
				}
				if aggregatedLevels[lowerLevelType] && len(summaryTexts) <= 10 && !isManual {
					// From aggregated level with small number of summaries: direct merge
					// Manual generation always uses LLM to regenerate the current level
					shouldDirectMerge = true
				}
			}

			var summaryResult string
			var err error

			if shouldDirectMerge {
				// Direct merge: simply combine the summaries with separators
				// This is fast and preserves all information without LLM overhead
				logger.GetLogger().Infof("Directly merging %d %s summaries for %s (no LLM processing)",
					len(summaryTexts), lowerLevelType, periodKey)
				summaryResult = strings.Join(summaryTexts, "\n\n---\n\n")
			} else if len(summaryTexts) == 1 {
				// Single summary, use regular summary
				summaryResult, err = e.analyzer.GenerateSummary(summaryTexts[0], periodType)
			} else if len(summaryTexts) == 2 {
				// Two summaries: equal merge instead of rolling
				// Rolling treats first as "previous context" and second as "new content"
				// which causes information loss when first is empty/idle
				combined := strings.Join(summaryTexts, "\n\n")
				summaryResult, err = e.analyzer.GenerateSummary(combined, periodType)
			} else {
				// 3+ summaries: combine all summaries and generate in one LLM call
				// No rolling summary - all summaries are merged and processed together
				combined := strings.Join(summaryTexts, "\n\n")
				summaryResult, err = e.analyzer.GenerateSummary(combined, periodType)
			}

			if err != nil {
				logger.GetLogger().Infof("WARNING: Failed to generate summary for %s: %v",
					periodKey, err)
				// Fallback: combine all summaries
				periodSummary = strings.Join(summaryTexts, "\n\n")
			} else {
				// For week and above, apply level-specific prompt to finalize the summary
				if periodType == "week" || periodType == "month" || periodType == "quarter" || periodType == "year" {
					finalSummary, finalErr := e.analyzer.GenerateSummary(summaryResult, periodType)
					if finalErr != nil {
						logger.GetLogger().Infof("WARNING: Failed to apply level-specific prompt for %s: %v, using summary result",
							periodKey, finalErr)
						periodSummary = summaryResult
					} else {
						periodSummary = finalSummary
					}
				} else {
					periodSummary = summaryResult
				}
			}
		} else {
			// No valid summaries found - check if we should generate a report
			// If no valid summaries and no screenshots, don't generate report
			// Update allScreenshotIDs with deduplicated IDs first to check
			allScreenshotIDs = nil
			for id := range screenshotIDSet {
				allScreenshotIDs = append(allScreenshotIDs, id)
			}

			if len(allScreenshotIDs) == 0 {
				logger.GetLogger().Infof("No valid summaries and no screenshots for %s (%s), skipping report generation",
					periodKey, periodType)
				return nil
			}

			// If we have screenshots but no valid summaries, set summary to empty
			// This will be handled by hasValidContent check later
			periodSummary = ""
		}

		// Update allScreenshotIDs with deduplicated IDs before saving (if not already updated)
		if len(allScreenshotIDs) == 0 {
			allScreenshotIDs = nil
			for id := range screenshotIDSet {
				allScreenshotIDs = append(allScreenshotIDs, id)
			}
		}

		// Clean summary if it indicates no work activity (remove efficiency analysis and improvement suggestions)
		periodSummary = cleanSummaryIfNoWorkActivity(periodSummary)

		// If summary is empty after cleaning and no screenshots, don't generate report
		if periodSummary == "" && len(allScreenshotIDs) == 0 {
			logger.GetLogger().Infof("No valid content and no screenshots for %s (%s), skipping report generation",
				periodKey, periodType)
			return nil
		}

		// Generate analysis only for week and longer periods
		// Day and below focus on factual records only
		// Only generate analysis if there is valid work activity
		if periodSummary != "" && len(summaryTexts) > 0 && shouldGenerateAnalysis(periodType) {
			if hasValidWorkActivity(periodSummary) {
				analysisResult, err := e.analyzer.AnalyzeBehavior(periodSummary)
				if err != nil {
					logger.GetLogger().Infof("WARNING: Failed to perform improvement analysis for %s: %v",
						periodKey, err)
					improvementAnalysis = fmt.Sprintf("分析失败: %v", err)
				} else {
					improvementAnalysis = analysisResult
				}
			} else {
				logger.GetLogger().Infof("Skipping improvement analysis for %s: no valid work activity detected",
					periodKey)
				// Do not generate analysis if there is no valid work activity
				improvementAnalysis = ""
			}
		}

		// If we had lower-level summaries, we're done with aggregation
		if len(lowerSummaries) > 0 {
			// Already aggregated above, continue to save
		} else {
			// Fallback: aggregate from screenshots
			lowerLevelType = ""
		}
	}

	// If we need to aggregate from screenshots (either no lower-level type or fallback)
	if lowerLevelType == "" {
		// Aggregate directly from screenshots (for fifteenmin, or as fallback)
		screenshots, err := e.storage.QueryByDateRange(startTime, endTime)
		if err != nil {
			return fmt.Errorf("failed to query screenshots: %w", err)
		}

		if len(screenshots) == 0 {
			return nil
		}

		var screenshotSummaries []string
		for _, s := range screenshots {
			// Add screenshot IDs to deduplication set
			if s.ID != "" {
				screenshotIDSet[s.ID] = true
			}
			if s.Analysis != "" && !strings.HasPrefix(s.Analysis, "Analysis failed") {
				// Filter out desktop/lock screen screenshots
				if !isDesktopOrLockScreenAnalysis(s.Analysis) {
					screenshotSummaries = append(screenshotSummaries, s.Analysis)
				}
			}
		}

		// Update allScreenshotIDs with deduplicated IDs
		allScreenshotIDs = nil
		for id := range screenshotIDSet {
			allScreenshotIDs = append(allScreenshotIDs, id)
		}

		if len(screenshotSummaries) > 0 {
			rawSummaryText := strings.Join(screenshotSummaries, "\n")
			summaryResult, err := e.analyzer.GenerateSummary(rawSummaryText, periodType)
			if err != nil {
				logger.GetLogger().Infof("WARNING: Failed to generate summary for %s: %v",
					periodKey, err)
				periodSummary = rawSummaryText
			} else {
				periodSummary = summaryResult
			}
		} else {
			// If all screenshots were filtered out (desktop/lock screen), set summary to empty
			// No content should be generated when there's no work activity
			periodSummary = ""
		}

		// Clean summary if it indicates no work activity (remove efficiency analysis and improvement suggestions)
		periodSummary = cleanSummaryIfNoWorkActivity(periodSummary)

		// Generate analysis only for week and longer periods
		// Day and below focus on factual records only
		// Only generate analysis if there is valid work activity
		if periodSummary != "" && len(screenshotSummaries) > 0 && shouldGenerateAnalysis(periodType) {
			if hasValidWorkActivity(periodSummary) {
				analysisResult, err := e.analyzer.AnalyzeBehavior(periodSummary)
				if err != nil {
					logger.GetLogger().Infof("WARNING: Failed to perform improvement analysis for %s: %v",
						periodKey, err)
					improvementAnalysis = fmt.Sprintf("分析失败: %v", err)
				} else {
					improvementAnalysis = analysisResult
				}
			} else {
				logger.GetLogger().Infof("Skipping improvement analysis for %s: no valid work activity detected",
					periodKey)
				// Do not generate analysis if there is no valid work activity
				improvementAnalysis = ""
			}
		}
	}

	summary := &storage.PeriodSummary{
		PeriodKey:   periodKey,
		PeriodType:  periodType,
		StartTime:   startTime,
		EndTime:     endTime,
		Screenshots: strings.Join(allScreenshotIDs, ","),
		Summary:     periodSummary,
		Analysis:    improvementAnalysis,
	}

	// Check if summary has valid content before saving
	// If no valid content, save a placeholder to avoid re-checking in the future
	if !hasValidContent(summary) {
		// Save placeholder to mark that this period has been checked and has no work activity
		// This avoids re-checking the same period repeatedly when generating higher-level reports
		placeholderSummary := &storage.PeriodSummary{
			PeriodKey:   periodKey,
			PeriodType:  periodType,
			StartTime:   startTime,
			EndTime:     endTime,
			Screenshots: "", // No screenshots for placeholder
			Summary:     "__NO_WORK_ACTIVITY_PLACEHOLDER__",
			Analysis:    "",
		}

		if err := e.storage.SavePeriodSummary(placeholderSummary); err != nil {
			logger.GetLogger().Infof("WARNING: Failed to save placeholder for %s (%s): %v",
				periodKey, periodType, err)
		} else {
			logger.GetLogger().Infof("Saved placeholder for %s (%s): no valid work activity",
				periodKey, periodType)
		}

		// Don't save report file for placeholder
		return nil
	}

	if err := e.storage.SavePeriodSummary(summary); err != nil {
		return fmt.Errorf("failed to save period summary: %w", err)
	}

	// Save period summary as report file
	if err := e.savePeriodSummaryReport(summary); err != nil {
		logger.GetLogger().Infof("WARNING: Failed to save period summary report for %s: %v",
			periodKey, err)
	}

	logger.GetLogger().Infof("Period summary generated for %s (%s): %d screenshots",
		periodKey, periodType, len(allScreenshotIDs))

	return nil
}

// determineActualTimeRange queries actual data (screenshots or lower-level summaries) to determine the real time range
// Returns actual start time, actual end time, and whether data exists
func (e *Executor) determineActualTimeRange(periodType string, theoreticalStart, theoreticalEnd time.Time) (time.Time, time.Time, bool) {
	lowerLevelType := e.getLowerLevelPeriodType(periodType)

	var earliestTime, latestTime time.Time
	hasData := false

	if lowerLevelType != "" {
		// Try to query lower-level summaries first
		lowerSummaries, err := e.storage.QueryPeriodSummaries(lowerLevelType, theoreticalStart, theoreticalEnd)
		if err == nil && len(lowerSummaries) > 0 {
			// Found lower-level summaries, use their time range
			earliestTime = lowerSummaries[0].StartTime
			latestTime = lowerSummaries[0].EndTime
			hasData = true

			for _, s := range lowerSummaries {
				if s.StartTime.Before(earliestTime) {
					earliestTime = s.StartTime
				}
				if s.EndTime.After(latestTime) {
					latestTime = s.EndTime
				}
			}
		}
	}

	// If no lower-level summaries found, query screenshots directly
	if !hasData {
		screenshots, err := e.storage.QueryByDateRange(theoreticalStart, theoreticalEnd)
		if err == nil && len(screenshots) > 0 {
			// Filter by work hours for relevant period types
			if periodType == "hour" || periodType == "day" {
				screenshots = e.filterWorkTimeScreenshots(screenshots)
			}

			if len(screenshots) > 0 {
				earliestTime = screenshots[0].Timestamp
				latestTime = screenshots[0].Timestamp
				hasData = true

				for _, s := range screenshots {
					if s.Timestamp.Before(earliestTime) {
						earliestTime = s.Timestamp
					}
					if s.Timestamp.After(latestTime) {
						latestTime = s.Timestamp
					}
				}
			}
		}
	}

	if !hasData {
		return theoreticalStart, theoreticalEnd, false
	}

	// Round to period boundaries based on period type
	switch periodType {
	case "day":
		actualStart := time.Date(earliestTime.Year(), earliestTime.Month(), earliestTime.Day(), 0, 0, 0, 0, earliestTime.Location())
		actualEnd := time.Date(latestTime.Year(), latestTime.Month(), latestTime.Day(), 23, 59, 59, 0, latestTime.Location())
		return actualStart, actualEnd, true
	case "hour":
		actualStart := time.Date(earliestTime.Year(), earliestTime.Month(), earliestTime.Day(), earliestTime.Hour(), 0, 0, 0, earliestTime.Location())
		actualEnd := time.Date(latestTime.Year(), latestTime.Month(), latestTime.Day(), latestTime.Hour(), 59, 59, 0, latestTime.Location())
		return actualStart, actualEnd, true
	case "fifteenmin":
		minute := earliestTime.Minute()
		roundedMinute := (minute / 15) * 15
		actualStart := time.Date(earliestTime.Year(), earliestTime.Month(), earliestTime.Day(), earliestTime.Hour(), roundedMinute, 0, 0, earliestTime.Location())
		minute = latestTime.Minute()
		roundedMinute = (minute / 15) * 15
		actualEnd := time.Date(latestTime.Year(), latestTime.Month(), latestTime.Day(), latestTime.Hour(), roundedMinute+14, 59, 0, latestTime.Location())
		return actualStart, actualEnd, true
	case "week":
		weekday := int(earliestTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		actualStart := time.Date(earliestTime.Year(), earliestTime.Month(), earliestTime.Day(), 0, 0, 0, 0, earliestTime.Location())
		actualStart = actualStart.AddDate(0, 0, -(weekday - 1))
		weekday = int(latestTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		actualEnd := time.Date(latestTime.Year(), latestTime.Month(), latestTime.Day(), 23, 59, 59, 0, latestTime.Location())
		actualEnd = actualEnd.AddDate(0, 0, 7-weekday)
		return actualStart, actualEnd, true
	case "month":
		actualStart := time.Date(earliestTime.Year(), earliestTime.Month(), 1, 0, 0, 0, 0, earliestTime.Location())
		actualEnd := time.Date(latestTime.Year(), latestTime.Month(), 1, 0, 0, 0, 0, latestTime.Location())
		actualEnd = actualEnd.AddDate(0, 1, 0).Add(-time.Second)
		return actualStart, actualEnd, true
	case "quarter":
		quarter := (int(earliestTime.Month())-1)/3 + 1
		quarterStartMonth := (quarter-1)*3 + 1
		actualStart := time.Date(earliestTime.Year(), time.Month(quarterStartMonth), 1, 0, 0, 0, 0, earliestTime.Location())
		quarter = (int(latestTime.Month())-1)/3 + 1
		quarterEndMonth := quarter * 3
		actualEnd := time.Date(latestTime.Year(), time.Month(quarterEndMonth), 1, 0, 0, 0, 0, latestTime.Location())
		actualEnd = actualEnd.AddDate(0, 1, 0).Add(-time.Second)
		return actualStart, actualEnd, true
	case "year":
		actualStart := time.Date(earliestTime.Year(), 1, 1, 0, 0, 0, 0, earliestTime.Location())
		actualEnd := time.Date(latestTime.Year(), 12, 31, 23, 59, 59, 0, latestTime.Location())
		return actualStart, actualEnd, true
	default:
		// For unknown types, use the theoretical range
		return theoreticalStart, theoreticalEnd, hasData
	}
}

// getLowerLevelPeriodType returns the lower-level period type for hierarchical aggregation
// Returns empty string if this is the lowest level (should aggregate from screenshots)
func (e *Executor) getLowerLevelPeriodType(periodType string) string {
	hierarchy := map[string]string{
		"year":         "quarter",
		"quarter":      "month",
		"month":        "week",
		"week":         "day",
		"day":          "work-segment",
		"work-segment": "hour",
		"hour":         "fifteenmin", // hour aggregates from four fifteenmin summaries
		"fifteenmin":   "",           // fifteenmin aggregates from screenshot analyses
	}
	return hierarchy[periodType]
}

// shouldGenerateAnalysis determines if a period type should generate behavior analysis
// Only week and longer periods have sufficient data for meaningful analysis
// Day and below focus on factual records only
func shouldGenerateAnalysis(periodType string) bool {
	switch periodType {
	case "week", "month", "quarter", "year":
		return true
	default:
		return false
	}
}

// isInvalidSummary checks if a summary is invalid (contains "no work activity" message)
// Invalid summaries should be regenerated from lower level
// Also checks for placeholder markers (which should not be regenerated, just skipped)
func isInvalidSummary(summary string) bool {
	if summary == "" {
		return true
	}

	// Check for placeholder marker - these are already checked and have no work activity
	// They should be skipped, not regenerated
	if summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
		return true
	}

	summaryLower := strings.ToLower(summary)
	invalidPatterns := []string{
		"该时间段内没有检测到有效工作活动（所有截图均为桌面或锁屏状态）",
		"该时间段内没有检测到有效工作活动",
		"没有检测到有效工作活动（所有截图均为桌面或锁屏状态）",
		"没有检测到有效工作活动",
		"没有有效工作活动",
		"未检测到新的有效工作活动",
		"未检测到有效工作活动",
	}

	for _, pattern := range invalidPatterns {
		if strings.Contains(summaryLower, strings.ToLower(pattern)) {
			// Check if this is essentially the only content in the summary
			normalized := strings.ToLower(summary)
			normalized = strings.ReplaceAll(normalized, "\n", " ")
			normalized = strings.ReplaceAll(normalized, "\r", " ")
			normalized = strings.ReplaceAll(normalized, "  ", " ")
			normalized = strings.ReplaceAll(normalized, "【", "")
			normalized = strings.ReplaceAll(normalized, "】", "")
			normalized = strings.ReplaceAll(normalized, "*", "")
			normalized = strings.ReplaceAll(normalized, "#", "")
			normalized = strings.TrimSpace(normalized)

			// Remove the pattern and check remaining content
			remaining := strings.ReplaceAll(normalized, strings.ToLower(pattern), "")
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

			// If after removing the pattern, there's little content left, it's invalid
			if len(remaining) < 50 {
				return true
			}
		}
	}

	return false
}

// cleanSummaryIfNoWorkActivity removes efficiency analysis and improvement suggestions
// from summary if it indicates no work activity
// Returns empty string if no work activity detected
func cleanSummaryIfNoWorkActivity(summary string) string {
	if summary == "" {
		return summary
	}

	// Check if summary indicates no work activity
	if !hasValidWorkActivity(summary) {
		// Return empty string - no content should be generated when there's no work activity
		return ""
	}

	// If summary is valid, check if it contains unwanted sections that should be removed
	// This handles cases where LLM didn't follow instructions properly
	summaryLower := strings.ToLower(summary)
	noWorkIndicators := []string{
		"没有检测到有效工作活动",
		"没有有效工作活动",
		"未检测到新的有效工作活动",
		"未检测到有效工作活动",
	}

	hasNoWorkIndicator := false
	for _, indicator := range noWorkIndicators {
		if strings.Contains(summaryLower, strings.ToLower(indicator)) {
			hasNoWorkIndicator = true
			break
		}
	}

	if hasNoWorkIndicator {
		// Remove 【效率分析】 and 【改进建议】 sections if they exist
		// Use regex to find and remove these sections
		lines := strings.Split(summary, "\n")
		var cleanedLines []string
		inEfficiencySection := false
		inImprovementSection := false

		for _, line := range lines {
			lineTrimmed := strings.TrimSpace(line)
			lineLower := strings.ToLower(lineTrimmed)

			// Check if we're entering efficiency analysis section
			if strings.Contains(lineLower, "【效率分析】") || strings.Contains(lineLower, "效率分析") {
				inEfficiencySection = true
				continue
			}

			// Check if we're entering improvement suggestions section
			if strings.Contains(lineLower, "【改进建议】") || strings.Contains(lineLower, "改进建议") {
				inImprovementSection = true
				continue
			}

			// Check if we're exiting these sections (new section starts)
			if strings.HasPrefix(lineTrimmed, "【") && !inEfficiencySection && !inImprovementSection {
				// New section starts, reset flags
				inEfficiencySection = false
				inImprovementSection = false
			}

			// Skip lines in efficiency or improvement sections
			if inEfficiencySection || inImprovementSection {
				continue
			}

			cleanedLines = append(cleanedLines, line)
		}

		cleanedSummary := strings.Join(cleanedLines, "\n")
		cleanedSummary = strings.TrimSpace(cleanedSummary)

		// If after cleaning, summary is very short or only contains no-work message,
		// return empty string - no content should be generated when there's no work activity
		if len(cleanedSummary) < 100 {
			return ""
		}

		return cleanedSummary
	}

	return summary
}

// hasValidWorkActivity checks if the summary indicates valid work activity
// Returns false if summary indicates no work activity (desktop/lock screen only)
func hasValidWorkActivity(summary string) bool {
	if summary == "" {
		return false
	}

	summaryLower := strings.ToLower(summary)
	summaryTrimmed := strings.TrimSpace(summary)

	// Check for indicators of no work activity
	noWorkIndicators := []string{
		"没有检测到有效工作活动",
		"没有有效工作活动",
		"未检测到新的有效工作活动",
		"未检测到有效工作活动",
		"no work activity",
		"没有检测到新的有效工作活动",
	}

	// Check for desktop/lock screen indicators combined with no work activity
	desktopLockScreenIndicators := []string{
		"桌面或锁屏状态",
		"桌面或锁屏",
		"均为桌面或锁屏",
		"所有截图内容均为桌面或锁屏状态",
		"desktop or lock screen",
		"lock screen",
		"desktop",
	}

	// First check: if summary contains any no-work-activity indicator
	for _, indicator := range noWorkIndicators {
		if strings.Contains(summaryLower, strings.ToLower(indicator)) {
			// Additional check: if summary is very short or only contains no-work message
			// Remove common markdown formatting and check length
			normalized := strings.ToLower(summaryTrimmed)
			normalized = strings.ReplaceAll(normalized, "\n", " ")
			normalized = strings.ReplaceAll(normalized, "\r", " ")
			normalized = strings.ReplaceAll(normalized, "  ", " ")
			normalized = strings.ReplaceAll(normalized, "【", "")
			normalized = strings.ReplaceAll(normalized, "】", "")
			normalized = strings.ReplaceAll(normalized, "*", "")
			normalized = strings.ReplaceAll(normalized, "#", "")
			normalized = strings.TrimSpace(normalized)

			// If summary is very short (less than 200 chars after normalization),
			// and contains no-work indicator, it's likely invalid
			if len(normalized) < 200 {
				return false
			}

			// If summary contains both no-work indicator and desktop/lock screen indicator,
			// it's definitely invalid
			for _, desktopIndicator := range desktopLockScreenIndicators {
				if strings.Contains(summaryLower, strings.ToLower(desktopIndicator)) {
					return false
				}
			}

			// If summary only contains the no-work message pattern (with minimal other content),
			// it's invalid
			remaining := normalized
			for _, indicator := range noWorkIndicators {
				remaining = strings.ReplaceAll(remaining, strings.ToLower(indicator), "")
			}
			for _, indicator := range desktopLockScreenIndicators {
				remaining = strings.ReplaceAll(remaining, strings.ToLower(indicator), "")
			}
			// Remove common punctuation and whitespace
			remaining = strings.ReplaceAll(remaining, "。", "")
			remaining = strings.ReplaceAll(remaining, ".", "")
			remaining = strings.ReplaceAll(remaining, "，", "")
			remaining = strings.ReplaceAll(remaining, ",", "")
			remaining = strings.ReplaceAll(remaining, "（", "")
			remaining = strings.ReplaceAll(remaining, "）", "")
			remaining = strings.ReplaceAll(remaining, "(", "")
			remaining = strings.ReplaceAll(remaining, ")", "")
			remaining = strings.ReplaceAll(remaining, " ", "")
			remaining = strings.TrimSpace(remaining)

			// If after removing no-work indicators, there's very little content left,
			// it's likely an invalid summary
			if len(remaining) < 50 {
				return false
			}
		}
	}

	// Second check: if summary contains desktop/lock screen indicators without substantial work content
	hasDesktopLockScreen := false
	for _, indicator := range desktopLockScreenIndicators {
		if strings.Contains(summaryLower, strings.ToLower(indicator)) {
			hasDesktopLockScreen = true
			break
		}
	}

	if hasDesktopLockScreen {
		// Check if summary has substantial work-related content
		workIndicators := []string{
			"代码", "开发", "编写", "调试", "测试", "部署", "提交", "修复",
			"项目", "任务", "工作", "完成", "实现", "优化", "设计",
			"code", "develop", "write", "debug", "test", "deploy", "commit", "fix",
			"project", "task", "work", "complete", "implement", "optimize", "design",
		}

		hasWorkContent := false
		for _, indicator := range workIndicators {
			if strings.Contains(summaryLower, strings.ToLower(indicator)) {
				hasWorkContent = true
				break
			}
		}

		// If summary mentions desktop/lock screen but has no work content, it's invalid
		if !hasWorkContent && len(summaryTrimmed) < 300 {
			return false
		}
	}

	return true
}

// getHigherLevelPeriodType returns the higher-level period type for upward aggregation
// Returns empty string if this is the highest level
func (e *Executor) getHigherLevelPeriodType(periodType string) string {
	hierarchy := map[string]string{
		"fifteenmin":   "hour", // fifteenmin aggregates to hour (4 fifteenmins = 1 hour)
		"hour":         "work-segment",
		"work-segment": "day",
		"day":          "week",
		"week":         "month",
		"month":        "quarter",
		"quarter":      "year",
		"year":         "", // year is the highest level
	}
	return hierarchy[periodType]
}

// getAllHigherLevelTypes returns all higher-level period types in order from current to highest
func (e *Executor) getAllHigherLevelTypes(periodType string) []string {
	var higherLevels []string
	current := periodType
	for {
		higher := e.getHigherLevelPeriodType(current)
		if higher == "" {
			break
		}
		higherLevels = append(higherLevels, higher)
		current = higher
	}
	return higherLevels
}

// isNetworkOrRateLimitError 检查是否是网络错误或限流错误
func isNetworkOrRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	networkErrors := []string{
		"timeout",
		"i/o timeout",
		"dial tcp",
		"connection refused",
		"connection reset",
		"rate limit",
		"status 429",
		"status 502",
		"status 503",
		"status 504",
		"no such host",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(strings.ToLower(errStr), netErr) {
			return true
		}
	}

	return false
}

// deleteExistingSummariesInRange deletes all period summaries of a specific type within a time range
func (e *Executor) deleteExistingSummariesInRange(periodType string, startTime, endTime time.Time) error {
	summaries, err := e.storage.QueryPeriodSummaries(periodType, startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to query %s summaries: %w", periodType, err)
	}

	logger.GetLogger().Infof("Found %d existing %s summaries to delete", len(summaries), periodType)
	for _, summary := range summaries {
		if err := e.storage.DeletePeriodSummary(summary.PeriodKey); err != nil {
			logger.GetLogger().Infof("WARNING: Failed to delete summary %s: %v", summary.PeriodKey, err)
		} else {
			logger.GetLogger().Infof("Deleted summary: %s", summary.PeriodKey)
		}
	}

	return nil
}

// generateHigherLevelSummaries generates all higher-level summaries from a given period type
// This allows starting from any level and aggregating upward
// All intermediate level reports will be updated
func (e *Executor) generateHigherLevelSummaries(startPeriodType string, startTime time.Time, forceFromScreenshots bool, isManual bool) error {
	higherLevels := e.getAllHigherLevelTypes(startPeriodType)
	if len(higherLevels) == 0 {
		logger.GetLogger().Infof("No higher-level summaries to generate from %s", startPeriodType)
		return nil
	}

	logger.GetLogger().Infof("Generating higher-level summaries from %s: %v", startPeriodType, higherLevels)

	// Generate each higher level in order
	for _, higherLevelType := range higherLevels {
		// Determine the time range for this higher level based on startTime
		var periodTime time.Time
		switch higherLevelType {
		case "work-segment":
			// Work-segment is per day, use the day of startTime
			periodTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
		case "day":
			periodTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
		case "week":
			weekday := int(startTime.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			periodTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
			periodTime = periodTime.AddDate(0, 0, -(weekday - 1))
		case "month":
			periodTime = time.Date(startTime.Year(), startTime.Month(), 1, 0, 0, 0, 0, startTime.Location())
		case "year":
			periodTime = time.Date(startTime.Year(), 1, 1, 0, 0, 0, 0, startTime.Location())
		default:
			logger.GetLogger().Infof("WARNING: Unsupported higher-level type %s, skipping", higherLevelType)
			continue
		}

		// Generate this higher-level summary
		// This will automatically generate lower-level summaries if needed
		// and will save both database and report file
		if err := e.generateSinglePeriodSummary(periodTime, higherLevelType, forceFromScreenshots, isManual); err != nil {
			logger.GetLogger().Infof("WARNING: Failed to generate %s summary from %s: %v",
				higherLevelType, startPeriodType, err)
			// Continue with next level even if this one fails
		} else {
			logger.GetLogger().Infof("Successfully generated %s summary from %s", higherLevelType, startPeriodType)
		}
	}

	return nil
}

// generateWorkSegmentSummary generates a work-segment summary for a specific day
// Work-segment divides work hours (9:30-20:00) into multiple 2-hour segments
// Each segment aggregates from hour summaries
func (e *Executor) generateWorkSegmentSummary(dayStart time.Time, forceFromScreenshots bool) error {
	workHours := e.config.Screenshot.WorkHours
	startHour := workHours.StartHour
	startMinute := workHours.StartMinute
	endHour := workHours.EndHour
	endMinute := workHours.EndMinute

	// Calculate work start and end time for this day
	workStart := time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), startHour, startMinute, 0, 0, dayStart.Location())
	workEnd := time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), endHour, endMinute, 0, 0, dayStart.Location())

	// Divide work hours into 2-hour segments
	segmentDuration := 2 * time.Hour
	segments := []struct {
		start time.Time
		end   time.Time
		key   string
	}{}

	current := workStart
	segmentIndex := 0
	for current.Before(workEnd) {
		segmentEnd := current.Add(segmentDuration)
		if segmentEnd.After(workEnd) {
			segmentEnd = workEnd
		}

		// Format: YYYY-MM-DD-segment-N (e.g., 2025-11-21-segment-0)
		segmentKey := fmt.Sprintf("%s-segment-%d", dayStart.Format("2006-01-02"), segmentIndex)

		segments = append(segments, struct {
			start time.Time
			end   time.Time
			key   string
		}{
			start: current,
			end:   segmentEnd,
			key:   segmentKey,
		})

		current = segmentEnd
		segmentIndex++
	}

	// Generate summaries for each segment
	for _, segment := range segments {
		// Check if segment summary already exists
		existing, err := e.storage.GetPeriodSummary(segment.key)
		if err != nil {
			logger.GetLogger().Infof("WARNING: Failed to check work-segment summary %s: %v",
				segment.key, err)
		} else if existing == nil || forceFromScreenshots {
			// Query hour summaries within this segment (only work hours)
			hourSummaries, err := e.storage.QueryPeriodSummaries("hour", segment.start, segment.end)
			if err != nil {
				logger.GetLogger().Infof("WARNING: Failed to query hour summaries for segment %s: %v",
					segment.key, err)
				continue
			}

			// Filter hour summaries to only include those within work hours
			var workHourSummaries []*storage.PeriodSummary
			for _, s := range hourSummaries {
				if e.config.Screenshot.WorkHours.IsWorkTime(s.StartTime) {
					workHourSummaries = append(workHourSummaries, s)
				}
			}

			if len(workHourSummaries) == 0 {
				logger.GetLogger().Infof("No work-time hour summaries found for segment %s, skipping",
					segment.key)
				continue
			}

			// Generate segment summary from hour summaries
			var summaryTexts []string
			var allScreenshotIDs []string
			for _, s := range workHourSummaries {
				if s.Summary != "" {
					summaryTexts = append(summaryTexts, s.Summary)
				}
				if s.Screenshots != "" {
					ids := strings.Split(s.Screenshots, ",")
					allScreenshotIDs = append(allScreenshotIDs, ids...)
				}
			}

			var periodSummary string
			if len(summaryTexts) > 0 {
				if len(summaryTexts) == 1 {
					periodSummary = summaryTexts[0]
				} else {
					// Combine all summaries and generate in one LLM call
					// No rolling summary - all summaries are merged and processed together
					combined := strings.Join(summaryTexts, "\n\n")
					generatedSummary, err := e.analyzer.GenerateSummary(combined, "work-segment")
					if err != nil {
						logger.GetLogger().Infof("WARNING: Failed to generate summary for segment %s: %v",
							segment.key, err)
						// Fallback: combine all summaries
						periodSummary = combined
					} else {
						periodSummary = generatedSummary
					}
				}
			} else {
				periodSummary = fmt.Sprintf("No work activity in segment %s", segment.key)
			}

			// Save segment summary
			summary := &storage.PeriodSummary{
				PeriodKey:   segment.key,
				PeriodType:  "work-segment",
				StartTime:   segment.start,
				EndTime:     segment.end,
				Screenshots: strings.Join(allScreenshotIDs, ","),
				Summary:     periodSummary,
				Analysis:    "", // Work-segment doesn't have behavior analysis
			}

			if err := e.storage.SavePeriodSummary(summary); err != nil {
				logger.GetLogger().Infof("WARNING: Failed to save work-segment summary %s: %v",
					segment.key, err)
				continue
			}

			// Save report file
			if err := e.savePeriodSummaryReport(summary); err != nil {
				logger.GetLogger().Infof("WARNING: Failed to save work-segment report for %s: %v",
					segment.key, err)
			}

			logger.GetLogger().Infof("Work-segment summary generated for %s: %d hour summaries",
				segment.key, len(workHourSummaries))
		}
	}

	return nil
}

// generateLowerLevelSummaries recursively generates all lower-level summaries for a given time range
func (e *Executor) generateLowerLevelSummaries(periodType string, startTime, endTime time.Time, forceFromScreenshots bool, isManual bool) error {
	switch periodType {
	case "fifteenmin":
		// Generate all fifteenmin summaries in the range
		current := startTime
		// Round down to nearest 15-minute boundary
		roundedMinute := (current.Minute() / 15) * 15
		current = time.Date(current.Year(), current.Month(), current.Day(), current.Hour(), roundedMinute, 0, 0, current.Location())

		totalFifteenmins := int(endTime.Sub(current) / (15 * time.Minute))
		if totalFifteenmins == 0 {
			totalFifteenmins = 1
		}

		logger.GetLogger().Infof("Generating %d fifteenmin summaries from %s to %s...",
			totalFifteenmins, startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"))

		// Collect all fifteenmin jobs
		type fifteenminJob struct {
			start time.Time
			end   time.Time
			key   string
			index int
		}

		var jobs []fifteenminJob
		jobIndex := 0
		for current.Before(endTime) {
			fifteenminEnd := current.Add(15 * time.Minute)
			if fifteenminEnd.After(endTime) {
				fifteenminEnd = endTime
			}
			fifteenminKey := current.Format("2006-01-02-15-04")

			// Check if summary already exists
			existing, err := e.storage.GetPeriodSummary(fifteenminKey)
			if err != nil {
				logger.GetLogger().Infof("WARNING: Failed to check fifteenmin summary %s: %v",
					fifteenminKey, err)
			}

			// Add to job list if needs generation
			if existing == nil || forceFromScreenshots {
				jobs = append(jobs, fifteenminJob{
					start: current,
					end:   fifteenminEnd,
					key:   fifteenminKey,
					index: jobIndex,
				})
			}

			current = fifteenminEnd
			jobIndex++
		}

		if len(jobs) == 0 {
			logger.GetLogger().Infof("All fifteenmin summaries already exist")
			return nil
		}

		// Determine worker count
		maxWorkers := e.config.Performance.MaxParallelFifteenmins
		if maxWorkers <= 0 {
			maxWorkers = 16 // Default to 16 parallel fifteenmins
		}
		if maxWorkers > len(jobs) {
			maxWorkers = len(jobs) // Don't create more workers than jobs
		}

		logger.GetLogger().Infof("Generating %d fifteenmin summaries with %d parallel workers",
			len(jobs), maxWorkers)

		// Generate fifteenmins in parallel
		semaphore := make(chan struct{}, maxWorkers)
		errChan := make(chan error, len(jobs))
		successChan := make(chan string, len(jobs))
		var wg sync.WaitGroup

		startTime := time.Now()
		completed := atomic.Int32{}

		for _, job := range jobs {
			wg.Add(1)
			go func(j fifteenminJob) {
				defer wg.Done()
				semaphore <- struct{}{}        // Acquire semaphore
				defer func() { <-semaphore }() // Release semaphore

				// Retry mechanism
				maxRetries := 3
				var generateErr error

				for retryAttempt := 0; retryAttempt < maxRetries; retryAttempt++ {
					if retryAttempt > 0 {
						waitTime := time.Duration(retryAttempt*30) * time.Second
						logger.GetLogger().Infof("Retrying fifteenmin %s (attempt %d/%d, waiting %v)",
							j.key, retryAttempt+1, maxRetries, waitTime)
						time.Sleep(waitTime)
					}

					generateErr = e.generateSinglePeriodSummary(j.start, "fifteenmin", forceFromScreenshots, isManual)
					if generateErr == nil {
						break
					}

					if retryAttempt < maxRetries-1 && isNetworkOrRateLimitError(generateErr) {
						logger.GetLogger().Infof("WARNING: Network/rate limit error for %s, will retry: %v",
							j.key, generateErr)
						continue
					}
				}

				if generateErr != nil {
					errChan <- fmt.Errorf("%s: %w", j.key, generateErr)
				} else {
					successChan <- j.key
					count := completed.Add(1)
					if count%10 == 0 || count == int32(len(jobs)) {
						elapsed := time.Since(startTime)
						rate := float64(count) / elapsed.Seconds()
						remaining := len(jobs) - int(count)
						eta := time.Duration(float64(remaining)/rate) * time.Second
						logger.GetLogger().Infof("Fifteenmin progress: %d/%d (%.1f%%), rate: %.1f/s, ETA: %v",
							count, len(jobs), float64(count)/float64(len(jobs))*100, rate, eta.Round(time.Second))
					}
				}
			}(job)
		}

		wg.Wait()
		close(errChan)
		close(successChan)

		// Collect results
		var errors []error
		for err := range errChan {
			errors = append(errors, err)
		}

		successCount := len(successChan)
		failCount := len(errors)

		elapsed := time.Since(startTime)
		logger.GetLogger().Infof("Fifteenmin generation completed: %d succeeded, %d failed, took %v",
			successCount, failCount, elapsed.Round(time.Second))

		if failCount > 0 {
			logger.GetLogger().Warnf("Failed to generate %d fifteenmin summaries", failCount)
			for _, err := range errors {
				logger.GetLogger().Warnf("  - %v", err)
			}
		}

		return nil
	case "hour":
		// Generate all hour summaries in the range
		current := startTime
		processed := 0
		totalHours := 0

		// Calculate total hours for progress tracking
		tempCurrent := startTime
		for tempCurrent.Before(endTime) {
			totalHours++
			tempCurrent = tempCurrent.Add(time.Hour)
		}

		logger.GetLogger().Infof("Generating %d hour summaries from %s to %s",
			totalHours, startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"))

		lastProgressTime := time.Now()
		for current.Before(endTime) {
			hourEnd := current.Add(time.Hour)
			if hourEnd.After(endTime) {
				hourEnd = endTime
			}
			// Check if summary already exists
			hourKey := current.Format("2006-01-02-15")
			existing, err := e.storage.GetPeriodSummary(hourKey)
			if err != nil {
				logger.GetLogger().Infof("WARNING: Failed to check hour summary %s: %v",
					hourKey, err)
			} else if existing == nil || forceFromScreenshots {
				// First generate all fifteenmin summaries for this hour
				if err := e.generateLowerLevelSummaries("fifteenmin", current, hourEnd, forceFromScreenshots, isManual); err != nil {
					logger.GetLogger().Infof("WARNING: Failed to generate fifteenmin summaries for hour %s: %v",
						hourKey, err)
				}
				// Then generate the hour summary
				logger.GetLogger().Infof("Generating hour summary %d/%d: %s",
					processed+1, totalHours, hourKey)
				if err := e.generateSinglePeriodSummary(current, "hour", forceFromScreenshots, isManual); err != nil {
					logger.GetLogger().Infof("WARNING: Failed to generate hour summary for %s: %v",
						hourKey, err)
				} else {
					logger.GetLogger().Infof("Hour summary %s completed", hourKey)
				}
			}
			processed++
			// Log progress every 30 seconds
			if time.Since(lastProgressTime) >= 30*time.Second {
				logger.GetLogger().Infof("Hour summaries progress: %d/%d (%.1f%%)",
					processed, totalHours, float64(processed)/float64(totalHours)*100)
				lastProgressTime = time.Now()
			}
			current = hourEnd
		}
		logger.GetLogger().Infof("All %d hour summaries completed", processed)
	case "work-segment":
		// Generate all work-segment summaries in the range
		current := startTime
		for current.Before(endTime) {
			dayStart := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
			dayEnd := dayStart.AddDate(0, 0, 1)
			if dayEnd.After(endTime) {
				dayEnd = endTime
			}
			// First generate all hour summaries for this day (only work hours)
			if err := e.generateLowerLevelSummaries("hour", dayStart, dayEnd, forceFromScreenshots, isManual); err != nil {
				logger.GetLogger().Infof("WARNING: Failed to generate hour summaries for work-segments: %v",
					err)
			}
			// Then generate work-segment summaries for this day
			if err := e.generateWorkSegmentSummary(dayStart, forceFromScreenshots); err != nil {
				logger.GetLogger().Infof("WARNING: Failed to generate work-segment summaries for day %s: %v",
					dayStart.Format("2006-01-02"), err)
			}
			current = dayEnd
		}
	case "day":
		// Generate all day summaries in the range
		current := startTime
		now := time.Now()
		for current.Before(endTime) {
			dayStart := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
			dayEnd := dayStart.AddDate(0, 0, 1)

			// Check if this day period is complete (has naturally ended)
			isComplete := dayEnd.Before(now) || dayEnd.Equal(now)

			if dayEnd.After(endTime) {
				dayEnd = endTime
				isComplete = false // Periods truncated by parent range are incomplete
			}

			dayKey := dayStart.Format("2006-01-02")

			if isComplete {
				// Complete period: generate natural period summary
				existing, err := e.storage.GetPeriodSummary(dayKey)
				if err != nil {
					logger.GetLogger().Infof("WARNING: Failed to check day summary %s: %v",
						dayKey, err)
				} else if existing == nil || forceFromScreenshots {
					if forceFromScreenshots {
						// Force rebuild: skip work-segment, generate from hour directly
						if err := e.generateLowerLevelSummaries("hour", dayStart, dayEnd, forceFromScreenshots, isManual); err != nil {
							logger.GetLogger().Infof("WARNING: Failed to generate hour summaries for day %s: %v",
								dayKey, err)
						}
					} else {
						// Normal flow: generate from work-segment
						if err := e.generateLowerLevelSummaries("work-segment", dayStart, dayEnd, forceFromScreenshots, isManual); err != nil {
							logger.GetLogger().Infof("WARNING: Failed to generate work-segment summaries for day %s: %v",
								dayKey, err)
						}
					}
					// Generate the natural day summary
					if err := e.generateSinglePeriodSummary(dayStart, "day", forceFromScreenshots, isManual); err != nil {
						logger.GetLogger().Infof("WARNING: Failed to generate day summary for %s: %v",
							dayKey, err)
					}
				}
			} else {
				// Incomplete period: generate summary based on actual data
				if err := e.generateSinglePeriodSummary(dayStart, "day", forceFromScreenshots, isManual); err != nil {
					logger.GetLogger().Infof("WARNING: Failed to generate day summary for %s: %v",
						dayKey, err)
				}
			}

			current = dayEnd
		}
	case "week":
		// Generate all week summaries in the range
		current := startTime
		now := time.Now()
		for current.Before(endTime) {
			weekday := int(current.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			weekStart := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
			weekStart = weekStart.AddDate(0, 0, -(weekday - 1))
			weekEnd := weekStart.AddDate(0, 0, 7)

			// Check if this week period is complete (has naturally ended)
			isComplete := weekEnd.Before(now) || weekEnd.Equal(now)

			if weekEnd.After(endTime) {
				weekEnd = endTime
				isComplete = false // Periods truncated by parent range are incomplete
			}

			weekKey := weekStart.Format("2006-01-02") + "-week"

			if isComplete {
				// Complete period: generate natural period summary
				existing, err := e.storage.GetPeriodSummary(weekKey)
				if err != nil {
					logger.GetLogger().Infof("WARNING: Failed to check week summary %s: %v",
						weekKey, err)
				} else if existing == nil || forceFromScreenshots {
					// First generate all day summaries for this week
					if err := e.generateLowerLevelSummaries("day", weekStart, weekEnd, forceFromScreenshots, isManual); err != nil {
						logger.GetLogger().Infof("WARNING: Failed to generate day summaries for week %s: %v",
							weekKey, err)
					}
					// Generate the natural week summary
					if err := e.generateSinglePeriodSummary(weekStart, "week", forceFromScreenshots, isManual); err != nil {
						logger.GetLogger().Infof("WARNING: Failed to generate week summary for %s: %v",
							weekKey, err)
					}
				}
			} else {
				// Incomplete period: generate summary based on actual data
				if err := e.generateSinglePeriodSummary(weekStart, "week", forceFromScreenshots, isManual); err != nil {
					logger.GetLogger().Infof("WARNING: Failed to generate week summary for %s: %v",
						weekKey, err)
				}
			}

			current = weekEnd
		}
	case "month":
		// Generate all month summaries in the range
		current := startTime
		now := time.Now()
		for current.Before(endTime) {
			monthStart := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, current.Location())
			monthEnd := monthStart.AddDate(0, 1, 0)

			// Check if this month period is complete (has naturally ended)
			isComplete := monthEnd.Before(now) || monthEnd.Equal(now)

			if monthEnd.After(endTime) {
				monthEnd = endTime
				isComplete = false // Periods truncated by parent range are incomplete
			}

			monthKey := monthStart.Format("2006-01")

			if isComplete {
				// Complete period: generate natural period summary
				existing, err := e.storage.GetPeriodSummary(monthKey)
				if err != nil {
					logger.GetLogger().Infof("WARNING: Failed to check month summary %s: %v",
						monthKey, err)
				} else if existing == nil || forceFromScreenshots {
					// First generate all week summaries for this month
					if err := e.generateLowerLevelSummaries("week", monthStart, monthEnd, forceFromScreenshots, isManual); err != nil {
						logger.GetLogger().Infof("WARNING: Failed to generate week summaries for month %s: %v",
							monthKey, err)
					}
					// Generate the natural month summary
					if err := e.generateSinglePeriodSummary(monthStart, "month", forceFromScreenshots, isManual); err != nil {
						logger.GetLogger().Infof("WARNING: Failed to generate month summary for %s: %v",
							monthKey, err)
					}
				}
			} else {
				// Incomplete period: generate summary based on actual data
				if err := e.generateSinglePeriodSummary(monthStart, "month", forceFromScreenshots, isManual); err != nil {
					logger.GetLogger().Infof("WARNING: Failed to generate month summary for %s: %v",
						monthKey, err)
				}
			}

			current = monthEnd
		}
	}
	return nil
}

func (e *Executor) updateHourSummary(record *storage.ScreenshotRecord) error {
	screenshots, err := e.storage.GetScreenshotsByHourKey(record.HourKey)
	if err != nil {
		return fmt.Errorf("failed to get screenshots: %w", err)
	}

	var ids []string
	var screenshotSummaries []string
	for _, s := range screenshots {
		ids = append(ids, s.ID)
		// s.Analysis contains factual description (semantically it's a summary)
		if s.Analysis != "" && s.Analysis != "Analysis failed" {
			// Filter out desktop/lock screen screenshots
			if !isDesktopOrLockScreenAnalysis(s.Analysis) {
				screenshotSummaries = append(screenshotSummaries, s.Analysis)
			}
		}
	}

	// Hour summary contains factual information only
	hourSummary := strings.Join(screenshotSummaries, "\n")
	if hourSummary == "" {
		hourSummary = fmt.Sprintf("Captured %d screenshot(s) during this hour", len(ids))
	}

	return e.storage.UpdateHourSummary(record.HourKey, ids, hourSummary)
}

func (e *Executor) saveReport(record *storage.ScreenshotRecord) error {
	if e.config.Storage.ReportsPath == "" {
		return nil // Reports path not configured, skip
	}

	// Ensure reports directory exists
	if err := e.config.Storage.EnsureReportsPath(); err != nil {
		return fmt.Errorf("failed to create reports directory: %w", err)
	}

	// Generate report content
	reportContent := e.generateReportContent(record)

	// 使用 StorageManager 保存报告（支持新的层级嵌套结构）
	relativePath, err := e.storageManager.SaveReport(record.Timestamp, reportContent)
	if err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	reportPath := filepath.Join(e.config.Storage.ReportsPath, relativePath)
	logger.GetLogger().Infof("Report saved: %s", reportPath)
	return nil
}

func (e *Executor) generateReportContent(record *storage.ScreenshotRecord) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# 截图分析报告\n\n")
	sb.WriteString(fmt.Sprintf("**时间**: %s\n\n", record.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**截图ID**: %s\n\n", record.ID))
	sb.WriteString(fmt.Sprintf("**截图路径**: %s\n\n", record.ImagePath))
	sb.WriteString(fmt.Sprintf("**屏幕ID**: %d\n\n", record.ScreenID))
	sb.WriteString("---\n\n")

	// Summary content: factual description of what user is doing
	if record.Analysis != "" && !strings.HasPrefix(record.Analysis, "Analysis failed") {
		sb.WriteString("## 事实总结\n\n")
		sb.WriteString(record.Analysis)
		sb.WriteString("\n\n")
	} else if record.Analysis != "" {
		sb.WriteString("## 事实总结\n\n")
		sb.WriteString("**生成失败**: ")
		sb.WriteString(record.Analysis)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## 事实总结\n\n")
		sb.WriteString("尚未生成\n\n")
	}

	// Footer
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("*报告生成时间: %s*\n", time.Now().Format("2006-01-02 15:04:05")))

	return sb.String()
}

// regenerateReportsForAnalyzedScreenshots regenerates reports for screenshots that have summary
// but might have outdated report files (e.g., generated before summary completed)
func (e *Executor) regenerateReportsForAnalyzedScreenshots(hourKey string) {
	// Get all screenshots for this hour that have analysis
	screenshots, err := e.storage.GetScreenshotsByHourKey(hourKey)
	if err != nil {
		logger.GetLogger().Infof("WARNING: Failed to get screenshots for report regeneration: %v",
			err)
		return
	}

	regenerated := 0
	for _, record := range screenshots {
		// Only regenerate if screenshot has summary but report might be outdated
		if record.Analysis != "" && !strings.HasPrefix(record.Analysis, "Analysis failed") {
			// Check if report exists and might be outdated
			yearDir := record.Timestamp.Format("2006")
			monthDir := record.Timestamp.Format("01")
			dayDir := record.Timestamp.Format("02")
			hourDir := record.Timestamp.Format("15")
			reportDir := filepath.Join(e.config.Storage.ReportsPath, yearDir, monthDir, dayDir, hourDir)
			// Filename only contains minute and second, since parent directory already has year/month/day/hour
			filename := fmt.Sprintf("%s.md", record.Timestamp.Format("04-05"))
			reportPath := filepath.Join(reportDir, filename)

			// Check if report exists and contains "尚未生成"
			if reportContent, err := os.ReadFile(reportPath); err == nil {
				if strings.Contains(string(reportContent), "尚未生成") {
					// Regenerate report with current summary
					if err := e.saveReport(record); err == nil {
						regenerated++
					}
				}
			}
		}
	}

	if regenerated > 0 {
		logger.GetLogger().Infof("Regenerated %d outdated reports for hour %s",
			regenerated, hourKey)
	}
}

// hasValidContent checks if a period summary has meaningful content
// Returns true if Summary or Analysis contains valid, non-empty content
// Placeholder summaries (marked with __NO_WORK_ACTIVITY_PLACEHOLDER__) are considered invalid
func hasValidContent(summary *storage.PeriodSummary) bool {
	// Check for placeholder marker - these mark periods that have been checked and have no work activity
	if summary.Summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
		return false
	}

	// Check Summary
	summaryValid := false
	if summary.Summary != "" {
		summaryText := strings.TrimSpace(summary.Summary)
		// Check if summary is just a simple "no work activity" message
		// Remove common markdown formatting and check if it's essentially just the no-activity message
		normalizedText := strings.ToLower(summaryText)
		normalizedText = strings.ReplaceAll(normalizedText, "\n", " ")
		normalizedText = strings.ReplaceAll(normalizedText, "\r", " ")
		normalizedText = strings.ReplaceAll(normalizedText, "  ", " ")
		normalizedText = strings.TrimSpace(normalizedText)

		// Check if summary is essentially just "no work activity" message
		// This covers cases like:
		// - "该时间段内没有检测到有效工作活动（所有截图均为桌面或锁屏状态）"
		// - "该时间段内没有检测到有效工作活动。"
		// - "No work activity in segment X"
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
				// Check if the text is essentially just this message (maybe with some punctuation or parentheses)
				// Remove the pattern and common punctuation, check if there's substantial content left
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

				// If after removing the pattern and punctuation, there's little left, it's invalid
				// But if there's substantial content (like detailed analysis), it's valid
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
		// Check if analysis indicates failure
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

// calculateReportPath calculates the report file path for a period summary
func (e *Executor) calculateReportPath(summary *storage.PeriodSummary) (string, error) {
	if e.config.Storage.ReportsPath == "" {
		return "", fmt.Errorf("reports path not configured")
	}

	var summaryDir string
	var filename string
	periodType := summary.PeriodType

	switch periodType {
	case "year":
		yearDir := summary.StartTime.Format("2006")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir)
		filename = "year.md"
	case "quarter":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir)
		filename = fmt.Sprintf("quarter-Q%d.md", quarter)
	case "month":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir)
		filename = "month.md"
	case "week":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir)
		// 使用Calendar Week（月内周号）
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		filename = fmt.Sprintf("week-W%d.md", weekNum)
	case "work-segment":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		// 计算Calendar Week
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		weekDir := fmt.Sprintf("W%d", weekNum)
		dayDir := summary.StartTime.Format("02")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir)
		// Extract segment index from period key (format: YYYY-MM-DD-segment-N)
		// Use period key directly as filename
		parts := strings.Split(summary.PeriodKey, "-")
		if len(parts) >= 4 && parts[3] == "segment" {
			segmentNum := parts[4]
			filename = fmt.Sprintf("work-segment-%s.md", segmentNum)
		} else {
			filename = fmt.Sprintf("%s.md", summary.PeriodKey)
		}
	case "day":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		// 计算Calendar Week
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		weekDir := fmt.Sprintf("W%d", weekNum)
		dayDir := summary.StartTime.Format("02")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir)
		filename = "day.md"
	case "hour":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		// 计算Calendar Week
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		weekDir := fmt.Sprintf("W%d", weekNum)
		dayDir := summary.StartTime.Format("02")
		hourDir := summary.StartTime.Format("15")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir, hourDir)
		filename = "hour.md"
	case "fifteenmin":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		// 计算Calendar Week
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		weekDir := fmt.Sprintf("W%d", weekNum)
		dayDir := summary.StartTime.Format("02")
		hourDir := summary.StartTime.Format("15")
		// Directory structure stops at hour level, minute info goes to filename
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir, hourDir)
		minute := summary.StartTime.Format("04")
		filename = fmt.Sprintf("fifteenmin-%s.md", minute)
	default:
		// For unknown types, use standard directory structure
		// This should not happen for standard period types, but handle gracefully
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		// 计算Calendar Week
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		weekDir := fmt.Sprintf("W%d", weekNum)
		dayDir := summary.StartTime.Format("02")
		summaryDir = filepath.Join(e.config.Storage.ReportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir)
		// Use period type as filename, not period key, to avoid generating files like "2025-11-19-day.md"
		filename = fmt.Sprintf("%s.md", summary.PeriodType)
	}

	reportPath := filepath.Join(summaryDir, filename)
	return reportPath, nil
}

// SavePeriodSummaryReport saves period summary as a report file
// This is a public wrapper for savePeriodSummaryReport
func (e *Executor) SavePeriodSummaryReport(summary *storage.PeriodSummary) error {
	return e.savePeriodSummaryReport(summary)
}

func (e *Executor) savePeriodSummaryReport(summary *storage.PeriodSummary) error {
	if e.config.Storage.ReportsPath == "" {
		return nil // Reports path not configured, skip
	}

	// Check if summary has valid content before saving
	if !hasValidContent(summary) {
		// Calculate report path to check if file exists and delete it
		reportPath, err := e.calculateReportPath(summary)
		if err != nil {
			// If path calculation fails, just skip without logging error
			return nil
		}

		// Delete existing report file if it exists
		if _, err := os.Stat(reportPath); err == nil {
			if err := os.Remove(reportPath); err == nil {
				logger.GetLogger().Infof("Deleted empty report file: %s", reportPath)
			}
		}

		logger.GetLogger().Infof("Skipping report generation for %s (%s): no valid content", summary.PeriodKey, summary.PeriodType)
		return nil
	}

	// Ensure reports directory exists
	if err := e.config.Storage.EnsureReportsPath(); err != nil {
		return fmt.Errorf("failed to create reports directory: %w", err)
	}

	// Calculate report path
	reportPath, err := e.calculateReportPath(summary)
	if err != nil {
		return fmt.Errorf("failed to calculate report path: %w", err)
	}

	// Extract directory from path for MkdirAll
	summaryDir := filepath.Dir(reportPath)
	if err := os.MkdirAll(summaryDir, 0755); err != nil {
		return fmt.Errorf("failed to create period summary directory: %w", err)
	}

	// Generate report content
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s周期总结报告\n\n", getPeriodTypeName(summary.PeriodType)))
	sb.WriteString(fmt.Sprintf("**周期类型**: %s\n\n", summary.PeriodType))
	sb.WriteString(fmt.Sprintf("**开始时间**: %s\n\n", summary.StartTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**结束时间**: %s\n\n", summary.EndTime.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**截图数量**: %d\n\n", len(strings.Split(summary.Screenshots, ","))))
	sb.WriteString("---\n\n")

	// Summary section: factual information
	sb.WriteString("## 事实总结\n\n")
	if summary.Summary != "" {
		sb.WriteString(summary.Summary)
	} else {
		sb.WriteString("暂无数据")
	}
	sb.WriteString("\n\n")

	// Analysis section: improvement suggestions
	// Only output analysis if there is valid work activity in the summary
	if summary.Analysis != "" && hasValidWorkActivity(summary.Summary) {
		sb.WriteString("---\n\n")
		sb.WriteString("## 改进建议\n\n")
		sb.WriteString(summary.Analysis)
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("*报告生成时间: %s*\n", time.Now().Format("2006-01-02 15:04:05")))

	// Write report to file
	if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write period summary report file: %w", err)
	}

	logger.GetLogger().Infof("Period summary report saved: %s", reportPath)
	return nil
}

func getPeriodTypeName(periodType string) string {
	switch periodType {
	case "hour":
		return "小时"
	case "work-segment":
		return "工作时间段"
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

// processRollingSummary processes a list of summaries using rolling summary technique
// Returns the final rolled summary and any error encountered
func (e *Executor) processRollingSummary(summaries []string, periodKey string, context string) (string, error) {
	return e.processRollingSummaryWithTimeContext(summaries, periodKey, context, "")
}

// processRollingSummaryWithTimeContext processes rolling summary with time context for progress logging
func (e *Executor) processRollingSummaryWithTimeContext(summaries []string, periodKey string, context string, timeContext string) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no summaries to process")
	}

	if len(summaries) == 1 {
		return summaries[0], nil
	}

	// For large datasets, use hierarchical tree aggregation instead of linear rolling
	// This reduces API calls from O(n) to O(log n)
	const treeAggregationThreshold = 20
	if len(summaries) > treeAggregationThreshold {
		logger.GetLogger().Infof("Using tree aggregation for %d summaries (threshold: %d)",
			len(summaries), treeAggregationThreshold)
		return e.processTreeAggregationWithTimeContext(summaries, periodKey, context, timeContext)
	}

	// For smaller datasets, use traditional linear rolling
	sessionID := uuid.New().String()
	previousSummary := summaries[0]
	lastProgressTime := time.Now()
	totalSteps := len(summaries) - 1

	logger.GetLogger().Infof("Starting linear rolling summary session %s for %s (%s): %d steps",
		sessionID, periodKey, context, totalSteps)

	for i := 1; i < len(summaries); i++ {
		// Log progress every 10 steps or every 30 seconds
		if i%10 == 0 || time.Since(lastProgressTime) >= 30*time.Second {
			logger.GetLogger().Infof("Rolling summary progress for %s (%s): %d/%d (%.1f%%)",
				periodKey, context, i, totalSteps, float64(i)/float64(totalSteps)*100)
			lastProgressTime = time.Now()
		}

		newContent := summaries[i]
		rolledSummary, err := e.analyzer.GenerateRollingSummaryWithContext(previousSummary, newContent, timeContext)
		if err != nil {
			return "", fmt.Errorf("failed at step %d: %w", i, err)
		}
		previousSummary = rolledSummary
	}

	logger.GetLogger().Infof("Completed linear rolling summary session %s for %s (%s)",
		sessionID, periodKey, context)

	return previousSummary, nil
}

// processTreeAggregation uses hierarchical tree aggregation for large datasets
// This reduces API calls from O(n) to O(log n), dramatically improving performance
// Example: 100 items with linear rolling = 99 API calls (~33 min)
//
//	100 items with tree aggregation = ~14 API calls (~5 min)
func (e *Executor) processTreeAggregation(summaries []string, periodKey string, context string) (string, error) {
	return e.processTreeAggregationWithTimeContext(summaries, periodKey, context, "")
}

// processTreeAggregationWithTimeContext uses tree aggregation with time context for progress logging
func (e *Executor) processTreeAggregationWithTimeContext(summaries []string, periodKey string, context string, timeContext string) (string, error) {
	sessionID := uuid.New().String()
	currentLevel := summaries
	level := 0

	logger.GetLogger().Infof("Starting tree aggregation session %s for %s (%s): %d items",
		sessionID, periodKey, context, len(summaries))

	// Determine worker count for parallel processing
	maxWorkers := e.config.Performance.MaxParallelTreeAggregation
	if maxWorkers <= 0 {
		maxWorkers = 10 // Default to 10 parallel pairs
	}

	// Aggregate in levels: combine pairs at each level until we reach a single result
	for len(currentLevel) > 1 {
		level++
		pairsInLevel := (len(currentLevel) + 1) / 2

		if timeContext != "" {
			logger.GetLogger().Infof("Tree aggregation level %d for %s: processing %d items into %d pairs (%s) with %d parallel workers",
				level, periodKey, len(currentLevel), pairsInLevel, timeContext, maxWorkers)
		} else {
			logger.GetLogger().Infof("Tree aggregation level %d for %s: processing %d items into %d pairs with %d parallel workers",
				level, periodKey, len(currentLevel), pairsInLevel, maxWorkers)
		}

		// Adjust maxWorkers if we have fewer pairs than workers
		actualWorkers := maxWorkers
		if actualWorkers > pairsInLevel {
			actualWorkers = pairsInLevel
		}

		// Process pairs in parallel: [0,1], [2,3], [4,5], ...
		type pairResult struct {
			index    int
			combined string
		}

		resultChan := make(chan pairResult, pairsInLevel)
		semaphore := make(chan struct{}, actualWorkers)
		var wg sync.WaitGroup
		completed := atomic.Int32{}

		// Process all pairs
		for i := 0; i < len(currentLevel); i += 2 {
			if i+1 < len(currentLevel) {
				wg.Add(1)
				pairIndex := i
				go func() {
					defer wg.Done()
					semaphore <- struct{}{}        // Acquire semaphore
					defer func() { <-semaphore }() // Release semaphore

					// We have a pair, combine them
					combined, err := e.analyzer.GenerateRollingSummaryWithContext(currentLevel[pairIndex], currentLevel[pairIndex+1], timeContext)
					if err != nil {
						logger.GetLogger().Warnf("Tree aggregation failed at level %d, pair [%d,%d]: %v, using concatenation fallback",
							level, pairIndex, pairIndex+1, err)
						// Fallback: simple concatenation
						combined = currentLevel[pairIndex] + "\n\n" + currentLevel[pairIndex+1]
					}

					resultChan <- pairResult{
						index:    pairIndex / 2,
						combined: combined,
					}

					// Log progress
					count := completed.Add(1)
					if count%10 == 0 || count == int32(pairsInLevel) {
						logger.GetLogger().Infof("Tree aggregation level %d progress: %d/%d pairs (%.1f%%)",
							level, count, pairsInLevel, float64(count)/float64(pairsInLevel)*100)
					}
				}()
			} else {
				// Odd item out, carry to next level unchanged
				resultChan <- pairResult{
					index:    i / 2,
					combined: currentLevel[i],
				}
			}
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(resultChan)

		// Collect results in order
		nextLevel := make([]string, pairsInLevel)
		for result := range resultChan {
			nextLevel[result.index] = result.combined
		}

		currentLevel = nextLevel
		logger.GetLogger().Infof("Tree aggregation level %d completed: %d items remain", level, len(currentLevel))
	}

	if len(currentLevel) == 0 {
		return "", fmt.Errorf("tree aggregation resulted in empty output")
	}

	logger.GetLogger().Infof("Completed tree aggregation session %s for %s (%s) in %d levels",
		sessionID, periodKey, context, level)

	return currentLevel[0], nil
}

// filterWorkTimeScreenshots filters screenshots to only include those within work hours
func (e *Executor) filterWorkTimeScreenshots(screenshots []*storage.ScreenshotRecord) []*storage.ScreenshotRecord {
	var filtered []*storage.ScreenshotRecord
	for _, s := range screenshots {
		if e.config.Screenshot.WorkHours.IsWorkTime(s.Timestamp) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// CheckAndFillMissingSummaries checks for missing period summaries and fills them
// This reduces token consumption by ensuring all intermediate summaries are saved
// Checks the last N days (default 7 days) for missing summaries at all levels
func (e *Executor) CheckAndFillMissingSummaries(daysBack int) error {
	if daysBack <= 0 {
		daysBack = 7 // Default to 7 days
	}

	now := time.Now()
	startTime := now.AddDate(0, 0, -daysBack)
	endTime := now

	logger.GetLogger().Infof("Checking for missing summaries from %s to %s (last %d days)",
		startTime.Format("2006-01-02"), endTime.Format("2006-01-02"), daysBack)

	// Check and fill summaries from bottom to top level
	levels := []string{"fifteenmin", "hour", "work-segment", "day"}

	for _, level := range levels {
		if err := e.checkAndFillLevel(level, startTime, endTime); err != nil {
			logger.GetLogger().Warnf("Failed to check and fill %s summaries: %v", level, err)
			// Continue with other levels even if one fails
		}
	}

	logger.GetLogger().Info("Missing summaries check completed")
	return nil
}

// checkAndFillLevel checks for missing summaries at a specific level and fills them
func (e *Executor) checkAndFillLevel(periodType string, startTime, endTime time.Time) error {
	logger.GetLogger().Infof("Checking %s summaries from %s to %s",
		periodType, startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"))

	var missingCount int
	var filledCount int

	switch periodType {
	case "fifteenmin":
		// Check every 15 minutes in the range
		current := startTime
		// Round down to nearest 15-minute boundary
		roundedMinute := (current.Minute() / 15) * 15
		current = time.Date(current.Year(), current.Month(), current.Day(), current.Hour(), roundedMinute, 0, 0, current.Location())

		for current.Before(endTime) {
			periodEnd := current.Add(15 * time.Minute)
			if periodEnd.After(endTime) {
				periodEnd = endTime
			}

			periodKey := current.Format("2006-01-02-15-04")
			existing, err := e.storage.GetPeriodSummary(periodKey)
			if err != nil {
				logger.GetLogger().Warnf("Failed to check %s summary %s: %v", periodType, periodKey, err)
			} else if existing == nil {
				missingCount++
				// Check if we have screenshot analyses for this period
				screenshots, err := e.storage.QueryByDateRange(current, periodEnd)
				if err == nil && len(screenshots) > 0 {
					// We have screenshots, generate the summary
					if err := e.generateSinglePeriodSummary(current, periodType, false, false); err != nil {
						logger.GetLogger().Warnf("Failed to generate missing %s summary %s: %v", periodType, periodKey, err)
					} else {
						filledCount++
					}
				}
			}

			current = periodEnd
		}

	case "hour":
		// Check every hour in the range
		current := startTime
		current = time.Date(current.Year(), current.Month(), current.Day(), current.Hour(), 0, 0, 0, current.Location())

		for current.Before(endTime) {
			periodEnd := current.Add(time.Hour)
			if periodEnd.After(endTime) {
				periodEnd = endTime
			}

			periodKey := current.Format("2006-01-02-15")
			existing, err := e.storage.GetPeriodSummary(periodKey)
			if err != nil {
				logger.GetLogger().Warnf("Failed to check %s summary %s: %v", periodType, periodKey, err)
			} else if existing == nil {
				missingCount++
				// Generate missing hour summary (will auto-generate lower levels if needed)
				if err := e.generateSinglePeriodSummary(current, periodType, false, false); err != nil {
					logger.GetLogger().Warnf("Failed to generate missing %s summary %s: %v", periodType, periodKey, err)
				} else {
					filledCount++
				}
			}

			current = periodEnd
		}

	case "work-segment":
		// Check work-segments for each day in the range
		current := startTime
		current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())

		for current.Before(endTime) {
			dayEnd := current.AddDate(0, 0, 1)
			if dayEnd.After(endTime) {
				dayEnd = endTime
			}

			// Generate work-segment summaries for this day (will check and generate missing ones)
			if err := e.generateWorkSegmentSummary(current, false); err != nil {
				logger.GetLogger().Warnf("Failed to generate work-segment summaries for day %s: %v",
					current.Format("2006-01-02"), err)
			}

			current = dayEnd
		}
		// Work-segment generation already handles missing detection internally
		return nil

	case "day":
		// Check every day in the range
		current := startTime
		current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())

		for current.Before(endTime) {
			periodEnd := current.AddDate(0, 0, 1)
			if periodEnd.After(endTime) {
				periodEnd = endTime
			}

			periodKey := current.Format("2006-01-02")
			existing, err := e.storage.GetPeriodSummary(periodKey)
			if err != nil {
				logger.GetLogger().Warnf("Failed to check %s summary %s: %v", periodType, periodKey, err)
			} else if existing == nil {
				missingCount++
				// Generate missing day summary (will auto-generate lower levels if needed)
				if err := e.generateSinglePeriodSummary(current, periodType, false, false); err != nil {
					logger.GetLogger().Warnf("Failed to generate missing %s summary %s: %v", periodType, periodKey, err)
				} else {
					filledCount++
				}
			}

			current = periodEnd
		}
	}

	if missingCount > 0 {
		logger.GetLogger().Infof("%s: Found %d missing summaries, filled %d",
			periodType, missingCount, filledCount)
	} else {
		logger.GetLogger().Infof("%s: No missing summaries found", periodType)
	}

	return nil
}

// CleanupInvalidReports scans and deletes invalid report files
// This method should be called periodically by the daemon to maintain data quality
func (e *Executor) CleanupInvalidReports() error {
	if e.config.Storage.ReportsPath == "" {
		return fmt.Errorf("reports path not configured")
	}

	logger.GetLogger().Info("Starting invalid reports cleanup...")

	// Detect invalid reports
	issues, err := storage.DetectInvalidReports(e.config.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to scan reports: %w", err)
	}

	if len(issues) == 0 {
		logger.GetLogger().Info("No invalid reports found")
		return nil
	}

	// Group issues by category for logging
	issuesByCategory := make(map[string]int)
	for _, issue := range issues {
		issuesByCategory[issue.Category]++
	}

	logger.GetLogger().Infof("Found %d invalid reports:", len(issues))
	for category, count := range issuesByCategory {
		logger.GetLogger().Infof("  %s: %d", category, count)
	}

	// Get unique file paths (a file might have multiple issues)
	filePaths := make(map[string]bool)
	for _, issue := range issues {
		filePaths[issue.FilePath] = true
	}

	deletedCount := 0
	failedCount := 0

	// Delete invalid reports
	for filePath := range filePaths {
		// Extract period key from file path for database deletion
		parser := storage.NewReportParser(e.config.Storage.ReportsPath)
		parsed, err := parser.ParsePeriodReport(filePath)
		if err == nil && parsed != nil {
			periodType := parsed.PeriodType
			if periodType == "" {
				periodType = inferPeriodTypeFromPath(filePath)
			}

			if periodType != "" {
				periodKey, err := storage.ExtractPeriodKeyFromPath(filePath, periodType)
				if err == nil {
					// Delete from database
					if err := e.storage.DeletePeriodSummary(periodKey); err != nil {
						logger.GetLogger().Warnf("Failed to delete database record for %s: %v", periodKey, err)
					}
				}
			}
		}

		// Delete file
		if err := os.Remove(filePath); err != nil {
			logger.GetLogger().Warnf("Failed to delete invalid report %s: %v", filePath, err)
			failedCount++
		} else {
			logger.GetLogger().Infof("Deleted invalid report: %s", filePath)
			deletedCount++
		}
	}

	logger.GetLogger().Infof("Cleanup completed: deleted %d files, failed %d files", deletedCount, failedCount)

	return nil
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
	if len(parts) >= 4 {
		return "hour"
	}
	if len(parts) >= 3 {
		return "day"
	}
	if len(parts) >= 2 {
		return "month"
	}

	return ""
}
