package storage

import (
	"fmt"
	"time"
)

// ReportStorage implements a hybrid storage architecture for period reports
// following GitLab-style separation: metadata in database, content in filesystem
//
// Architecture:
// - metadataStorage (SQLite): Stores metadata, placeholders, and indexes
// - contentStorage (FileSystem): Stores actual report files
//
// This separation provides:
// - Fast metadata queries via database indexes
// - Efficient content storage via filesystem
// - Graceful degradation when files are manually deleted
type ReportStorage struct {
	metadataStorage *SQLiteStorage     // Metadata and placeholders (source of truth)
	contentStorage  *FileSystemStorage // Report file content
}

// SavePeriodSummary saves a period summary to database
// Placeholders (with Summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__") are saved to database only
// Valid summaries are saved to database (file saving is handled by executor.savePeriodSummaryReport)
// This separation ensures:
// - Metadata (including placeholders) is always in database
// - Report files are only generated for valid summaries via executor.savePeriodSummaryReport
func (r *ReportStorage) SavePeriodSummary(summary *PeriodSummary) error {
	// Always save to database for metadata (including placeholders)
	// File saving is handled separately by executor.savePeriodSummaryReport
	return r.metadataStorage.SavePeriodSummary(summary)
}

// GetPeriodSummary gets a period summary by period key
// Best practice: Database is the single source of truth for metadata
// 1. Query database first (fast, indexed)
// 2. If found and not a placeholder, enrich with file content if needed
// 3. Placeholders (no file) are returned directly from database
//
// Data consistency handling:
// - If file is manually deleted but database record exists: returns database version (graceful degradation)
// - This prevents orphaned database records from causing errors
// - To clean up orphaned records, use consistency check tools or manual cleanup
func (r *ReportStorage) GetPeriodSummary(periodKey string) (*PeriodSummary, error) {
	// Query database first (single source of truth for metadata)
	metadataSummary, err := r.metadataStorage.GetPeriodSummary(periodKey)
	if err != nil {
		return nil, err
	}
	if metadataSummary == nil {
		return nil, nil // Not found
	}

	// If it's a placeholder, return directly from database
	if metadataSummary.Summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
		return metadataSummary, nil
	}

	// For valid summaries, try to enrich with file content if file exists
	// If file was manually deleted, gracefully fall back to database version
	contentSummary, err := r.contentStorage.GetPeriodSummary(periodKey)
	if err != nil {
		// File read failed (e.g., file manually deleted), return database version
		// This prevents orphaned database records from causing errors
		return metadataSummary, nil
	}
	if contentSummary != nil {
		// File exists, use file content but preserve database metadata (screenshots, etc.)
		contentSummary.Screenshots = metadataSummary.Screenshots // Preserve screenshot IDs from database
		return contentSummary, nil
	}

	// File doesn't exist but database has record (orphaned data)
	// Return database version to maintain functionality
	// Note: This is expected behavior - database is source of truth, file is optional enrichment
	return metadataSummary, nil
}

// QueryPeriodSummaries queries period summaries by type and date range
// Best practice: Database is the single source of truth
// 1. Query database first (fast, indexed, includes placeholders)
// 2. For each result, enrich with file content if file exists
// 3. This ensures we get all records (including placeholders) efficiently
//
// Data consistency handling:
// - If files are manually deleted but database records exist: returns database versions (graceful degradation)
// - This prevents orphaned database records from causing errors
// - Database records are never lost even if files are deleted
func (r *ReportStorage) QueryPeriodSummaries(periodType string, start, end time.Time) ([]*PeriodSummary, error) {
	// Query database first (single source of truth, fast with indexes)
	metadataSummaries, err := r.metadataStorage.QueryPeriodSummaries(periodType, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query database: %w", err)
	}

	// Enrich with file content where files exist
	// If files are missing (manually deleted), gracefully fall back to database version
	var summaries []*PeriodSummary
	for _, metadataSummary := range metadataSummaries {
		// If it's a placeholder, return directly from database
		if metadataSummary.Summary == "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
			summaries = append(summaries, metadataSummary)
			continue
		}

		// For valid summaries, try to enrich with file content if file exists
		contentSummary, err := r.contentStorage.GetPeriodSummary(metadataSummary.PeriodKey)
		if err == nil && contentSummary != nil {
			// File exists, use file content but preserve database metadata
			contentSummary.Screenshots = metadataSummary.Screenshots // Preserve screenshot IDs from database
			summaries = append(summaries, contentSummary)
		} else {
			// File doesn't exist (may be manually deleted), use database version
			// This handles orphaned database records gracefully
			summaries = append(summaries, metadataSummary)
		}
	}

	// Results are already sorted by database query
	return summaries, nil
}

// DeletePeriodSummary deletes a period summary from both storage systems
// Best practice: Always delete from both to maintain consistency
// - Deletes database record (source of truth)
// - Deletes file if it exists (may have been manually deleted already)
// - Ignores errors if record/file doesn't exist (idempotent)
func (r *ReportStorage) DeletePeriodSummary(periodKey string) error {
	// Delete from both storage systems
	// Order: file first, then database (if file deletion fails, database deletion still proceeds)
	// This ensures database is always cleaned up even if file deletion fails
	contentErr := r.contentStorage.DeletePeriodSummary(periodKey)
	metadataErr := r.metadataStorage.DeletePeriodSummary(periodKey)

	// If database deletion fails, return error (database is source of truth)
	if metadataErr != nil {
		return fmt.Errorf("failed to delete from database: %w", metadataErr)
	}

	// If file deletion fails but database succeeded, log but don't fail
	// This handles cases where file was already manually deleted
	if contentErr != nil {
		// File deletion failed, but database deletion succeeded
		// This is acceptable - database is source of truth
		return nil
	}

	return nil
}

// Close closes both storage systems
func (r *ReportStorage) Close() error {
	if err := r.metadataStorage.Close(); err != nil {
		return err
	}
	return r.contentStorage.Close()
}

// Delegate screenshot and hour summary operations to metadataStorage
// These operations are database-only and don't involve report files

func (r *ReportStorage) SaveScreenshot(record *ScreenshotRecord) error {
	return r.metadataStorage.SaveScreenshot(record)
}

func (r *ReportStorage) UpdateScreenshotAnalysis(id, analysis string) error {
	return r.metadataStorage.UpdateScreenshotAnalysis(id, analysis)
}

func (r *ReportStorage) GetScreenshotsByHourKey(hourKey string) ([]*ScreenshotRecord, error) {
	return r.metadataStorage.GetScreenshotsByHourKey(hourKey)
}

func (r *ReportStorage) GetScreenshotsByIDs(ids []string) (map[string]*ScreenshotRecord, error) {
	return r.metadataStorage.GetScreenshotsByIDs(ids)
}

func (r *ReportStorage) GetHourSummary(hourKey string) (*HourSummary, error) {
	return r.metadataStorage.GetHourSummary(hourKey)
}

func (r *ReportStorage) SaveHourSummary(summary *HourSummary) error {
	return r.metadataStorage.SaveHourSummary(summary)
}

func (r *ReportStorage) UpdateHourSummary(hourKey string, screenshotIDs []string, summary string) error {
	return r.metadataStorage.UpdateHourSummary(hourKey, screenshotIDs, summary)
}

func (r *ReportStorage) QueryByDateRange(start, end time.Time) ([]*ScreenshotRecord, error) {
	return r.metadataStorage.QueryByDateRange(start, end)
}

func (r *ReportStorage) QueryHourSummariesByDateRange(start, end time.Time) ([]*HourSummary, error) {
	return r.metadataStorage.QueryHourSummariesByDateRange(start, end)
}

func (r *ReportStorage) GetUnanalyzedScreenshots(limit int) ([]*ScreenshotRecord, error) {
	return r.metadataStorage.GetUnanalyzedScreenshots(limit)
}

func (r *ReportStorage) CleanupOldRecords(retentionDays int) error {
	// Cleanup both storage systems
	if err := r.metadataStorage.CleanupOldRecords(retentionDays); err != nil {
		return err
	}
	return r.contentStorage.CleanupOldRecords(retentionDays)
}

func (r *ReportStorage) DeleteScreenshotsByIDs(ids []string) error {
	return r.metadataStorage.DeleteScreenshotsByIDs(ids)
}

func (r *ReportStorage) ClearAllSummaries() error {
	if err := r.metadataStorage.ClearAllSummaries(); err != nil {
		return err
	}
	return r.contentStorage.ClearAllSummaries()
}

func (r *ReportStorage) GetAllScreenshots() ([]*ScreenshotRecord, error) {
	return r.metadataStorage.GetAllScreenshots()
}

func (r *ReportStorage) RebuildFromDirectory(storagePath string, lockScreenDetector LockScreenDetector) (int, error) {
	// RebuildFromDirectory rebuilds screenshot data in database
	return r.metadataStorage.RebuildFromDirectory(storagePath, lockScreenDetector)
}
