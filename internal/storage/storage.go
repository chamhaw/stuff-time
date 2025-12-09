package storage

import (
	"fmt"
	"time"
)

// StorageInterface defines the storage interface
// Both SQLiteStorage and FileSystemStorage implement this interface
type StorageInterface interface {
	SaveScreenshot(record *ScreenshotRecord) error
	UpdateScreenshotAnalysis(id, analysis string) error
	GetScreenshotsByHourKey(hourKey string) ([]*ScreenshotRecord, error)
	GetScreenshotsByIDs(ids []string) (map[string]*ScreenshotRecord, error)
	GetHourSummary(hourKey string) (*HourSummary, error)
	SaveHourSummary(summary *HourSummary) error
	UpdateHourSummary(hourKey string, screenshotIDs []string, summary string) error
	QueryByDateRange(start, end time.Time) ([]*ScreenshotRecord, error)
	QueryHourSummariesByDateRange(start, end time.Time) ([]*HourSummary, error)
	GetUnanalyzedScreenshots(limit int) ([]*ScreenshotRecord, error)
	SavePeriodSummary(summary *PeriodSummary) error
	GetPeriodSummary(periodKey string) (*PeriodSummary, error)
	DeletePeriodSummary(periodKey string) error
	QueryPeriodSummaries(periodType string, start, end time.Time) ([]*PeriodSummary, error)
	CleanupOldRecords(retentionDays int) error
	DeleteScreenshotsByIDs(ids []string) error
	ClearAllSummaries() error
	GetAllScreenshots() ([]*ScreenshotRecord, error)
	Close() error
	RebuildFromDirectory(storagePath string, lockScreenDetector LockScreenDetector) (int, error)
}

// Storage is a type alias for backward compatibility
// It can be either *SQLiteStorage or *FileSystemStorage or *ReportStorage
type Storage struct {
	StorageInterface
}

// NewStorage creates a storage instance
// If reportsPath is provided, creates a ReportStorage that uses:
// - metadataStorage (SQLite): Stores metadata, placeholders, and indexes
// - contentStorage (FileSystem): Stores actual report files
func NewStorage(dbPath string, reportsPath ...string) (*Storage, error) {
	// If reportsPath is provided, create report storage (hybrid architecture)
	if len(reportsPath) > 0 && reportsPath[0] != "" {
		contentStorage, err := NewFileSystemStorage(reportsPath[0])
		if err != nil {
			return nil, fmt.Errorf("failed to create file system storage: %w", err)
		}

		// Also create SQLite storage for metadata and placeholders
		metadataStorage, err := NewSQLiteStorage(dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite storage: %w", err)
		}

		reportStorage := &ReportStorage{
			metadataStorage: metadataStorage,
			contentStorage:  contentStorage,
		}
		return &Storage{StorageInterface: reportStorage}, nil
	}

	// Otherwise, use SQLite storage (for backward compatibility)
	sqliteStorage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite storage: %w", err)
	}
	return &Storage{StorageInterface: sqliteStorage}, nil
}

// NewSQLiteStorage creates a SQLite storage instance
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	return newSQLiteStorage(dbPath)
}
