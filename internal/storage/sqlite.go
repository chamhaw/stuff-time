package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStorage struct {
	db *sql.DB
}

// newSQLiteStorage creates a SQLite storage instance (internal function)
func newSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &SQLiteStorage{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) init() error {
	createScreenshotsTable := `
	CREATE TABLE IF NOT EXISTS screenshots (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		screen_id INTEGER NOT NULL,
		image_path TEXT NOT NULL,
		analysis TEXT,
		hour_key TEXT NOT NULL
	);
	`

	createHourSummariesTable := `
	CREATE TABLE IF NOT EXISTS hour_summaries (
		hour_key TEXT PRIMARY KEY,
		date DATE NOT NULL,
		hour INTEGER NOT NULL,
		screenshots TEXT NOT NULL,
		summary TEXT
	);
	`

	createPeriodSummariesTable := `
	CREATE TABLE IF NOT EXISTS period_summaries (
		period_key TEXT PRIMARY KEY,
		period_type TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		screenshots TEXT NOT NULL,
		summary TEXT,
		analysis TEXT
	);
	`

	createIndexes := `
	CREATE INDEX IF NOT EXISTS idx_screenshots_timestamp ON screenshots(timestamp);
	CREATE INDEX IF NOT EXISTS idx_screenshots_hour_key ON screenshots(hour_key);
	CREATE INDEX IF NOT EXISTS idx_hour_summaries_date ON hour_summaries(date);
	CREATE INDEX IF NOT EXISTS idx_period_summaries_type ON period_summaries(period_type);
	CREATE INDEX IF NOT EXISTS idx_period_summaries_start ON period_summaries(start_time);
	`

	if _, err := s.db.Exec(createScreenshotsTable); err != nil {
		return fmt.Errorf("failed to create screenshots table: %w", err)
	}

	if _, err := s.db.Exec(createHourSummariesTable); err != nil {
		return fmt.Errorf("failed to create hour_summaries table: %w", err)
	}

	if _, err := s.db.Exec(createPeriodSummariesTable); err != nil {
		return fmt.Errorf("failed to create period_summaries table: %w", err)
	}

	if _, err := s.db.Exec(createIndexes); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) SaveScreenshot(record *ScreenshotRecord) error {
	query := `
	INSERT INTO screenshots (id, timestamp, screen_id, image_path, analysis, hour_key)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, record.ID, record.Timestamp.Format(time.RFC3339Nano), record.ScreenID, record.ImagePath, record.Analysis, record.HourKey)
	if err != nil {
		return fmt.Errorf("failed to save screenshot: %w", err)
	}
	return nil
}

// UpdateScreenshotAnalysis updates the summary field (semantically, analysis stores summary)
func (s *SQLiteStorage) UpdateScreenshotAnalysis(id, analysis string) error {
	query := `UPDATE screenshots SET analysis = ? WHERE id = ?`
	_, err := s.db.Exec(query, analysis, id)
	if err != nil {
		return fmt.Errorf("failed to update screenshot summary: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetScreenshotsByHourKey(hourKey string) ([]*ScreenshotRecord, error) {
	query := `
	SELECT id, timestamp, screen_id, image_path, analysis, hour_key
	FROM screenshots
	WHERE hour_key = ?
	ORDER BY timestamp ASC
	`
	rows, err := s.db.Query(query, hourKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query screenshots: %w", err)
	}
	defer rows.Close()

	var records []*ScreenshotRecord
	for rows.Next() {
		var r ScreenshotRecord
		var timestampStr string
		if err := rows.Scan(&r.ID, &timestampStr, &r.ScreenID, &r.ImagePath, &r.Analysis, &r.HourKey); err != nil {
			return nil, fmt.Errorf("failed to scan screenshot: %w", err)
		}
		r.Timestamp, err = time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

// GetScreenshotsByIDs retrieves screenshot records by their IDs
func (s *SQLiteStorage) GetScreenshotsByIDs(ids []string) (map[string]*ScreenshotRecord, error) {
	if len(ids) == 0 {
		return make(map[string]*ScreenshotRecord), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
	SELECT id, timestamp, screen_id, image_path, analysis, hour_key
	FROM screenshots
	WHERE id IN (%s)
	ORDER BY timestamp ASC
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query screenshots: %w", err)
	}
	defer rows.Close()

	records := make(map[string]*ScreenshotRecord)
	for rows.Next() {
		var r ScreenshotRecord
		var timestampStr string
		if err := rows.Scan(&r.ID, &timestampStr, &r.ScreenID, &r.ImagePath, &r.Analysis, &r.HourKey); err != nil {
			return nil, fmt.Errorf("failed to scan screenshot: %w", err)
		}
		r.Timestamp, err = time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		records[r.ID] = &r
	}
	return records, rows.Err()
}

func (s *SQLiteStorage) GetHourSummary(hourKey string) (*HourSummary, error) {
	query := `
	SELECT hour_key, date, hour, screenshots, summary
	FROM hour_summaries
	WHERE hour_key = ?
	`
	var summary HourSummary
	var dateStr string
	err := s.db.QueryRow(query, hourKey).Scan(
		&summary.HourKey, &dateStr, &summary.Hour, &summary.Screenshots, &summary.Summary,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get hour summary: %w", err)
	}
	summary.Date, err = time.Parse(time.RFC3339Nano, dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}
	return &summary, nil
}

func (s *SQLiteStorage) SaveHourSummary(summary *HourSummary) error {
	query := `
	INSERT OR REPLACE INTO hour_summaries (hour_key, date, hour, screenshots, summary)
	VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, summary.HourKey, summary.Date.Format(time.RFC3339Nano), summary.Hour, summary.Screenshots, summary.Summary)
	if err != nil {
		return fmt.Errorf("failed to save hour summary: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) UpdateHourSummary(hourKey string, screenshotIDs []string, summary string) error {
	hourSummary, err := s.GetHourSummary(hourKey)
	if err != nil {
		return err
	}

	if hourSummary == nil {
		t, err := time.Parse("2006-01-02-15", hourKey)
		if err != nil {
			return fmt.Errorf("invalid hour key: %w", err)
		}
		hourSummary = &HourSummary{
			HourKey: hourKey,
			Date:    t,
			Hour:    t.Hour(),
		}
	}

	existingIDs := strings.Split(hourSummary.Screenshots, ",")
	idMap := make(map[string]bool)
	for _, id := range existingIDs {
		if id != "" {
			idMap[id] = true
		}
	}
	for _, id := range screenshotIDs {
		idMap[id] = true
	}

	var allIDs []string
	for id := range idMap {
		allIDs = append(allIDs, id)
	}
	hourSummary.Screenshots = strings.Join(allIDs, ",")
	hourSummary.Summary = summary

	return s.SaveHourSummary(hourSummary)
}

func (s *SQLiteStorage) QueryByDateRange(start, end time.Time) ([]*ScreenshotRecord, error) {
	query := `
	SELECT id, timestamp, screen_id, image_path, analysis, hour_key
	FROM screenshots
	WHERE timestamp >= ? AND timestamp <= ?
	ORDER BY timestamp ASC
	`
	rows, err := s.db.Query(query, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("failed to query screenshots: %w", err)
	}
	defer rows.Close()

	var records []*ScreenshotRecord
	for rows.Next() {
		var r ScreenshotRecord
		var timestampStr string
		if err := rows.Scan(&r.ID, &timestampStr, &r.ScreenID, &r.ImagePath, &r.Analysis, &r.HourKey); err != nil {
			return nil, fmt.Errorf("failed to scan screenshot: %w", err)
		}
		r.Timestamp, err = time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

func (s *SQLiteStorage) QueryHourSummariesByDateRange(start, end time.Time) ([]*HourSummary, error) {
	query := `
	SELECT hour_key, date, hour, screenshots, summary
	FROM hour_summaries
	WHERE date >= ? AND date <= ?
	ORDER BY date ASC, hour ASC
	`
	rows, err := s.db.Query(query, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("failed to query hour summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*HourSummary
	for rows.Next() {
		var s HourSummary
		var dateStr string
		if err := rows.Scan(&s.HourKey, &dateStr, &s.Hour, &s.Screenshots, &s.Summary); err != nil {
			return nil, fmt.Errorf("failed to scan hour summary: %w", err)
		}
		s.Date, err = time.Parse(time.RFC3339Nano, dateStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse date: %w", err)
		}
		summaries = append(summaries, &s)
	}
	return summaries, rows.Err()
}

// GetUnanalyzedScreenshots returns screenshots that don't have summary yet
// (semantically, analysis field stores summary of what user is doing)
func (s *SQLiteStorage) GetUnanalyzedScreenshots(limit int) ([]*ScreenshotRecord, error) {
	query := `
	SELECT id, timestamp, screen_id, image_path, analysis, hour_key
	FROM screenshots
	WHERE analysis IS NULL OR analysis = '' OR analysis LIKE 'Analysis failed%'
	ORDER BY timestamp ASC
	LIMIT ?
	`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query unanalyzed screenshots: %w", err)
	}
	defer rows.Close()

	var records []*ScreenshotRecord
	for rows.Next() {
		var r ScreenshotRecord
		var timestampStr string
		if err := rows.Scan(&r.ID, &timestampStr, &r.ScreenID, &r.ImagePath, &r.Analysis, &r.HourKey); err != nil {
			return nil, fmt.Errorf("failed to scan screenshot: %w", err)
		}
		r.Timestamp, err = time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

func (s *SQLiteStorage) SavePeriodSummary(summary *PeriodSummary) error {
	// Add analysis column if it doesn't exist (for backward compatibility)
	_, _ = s.db.Exec("ALTER TABLE period_summaries ADD COLUMN analysis TEXT")

	query := `
	INSERT OR REPLACE INTO period_summaries (period_key, period_type, start_time, end_time, screenshots, summary, analysis)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, summary.PeriodKey, summary.PeriodType, summary.StartTime.Format(time.RFC3339Nano), summary.EndTime.Format(time.RFC3339Nano), summary.Screenshots, summary.Summary, summary.Analysis)
	if err != nil {
		return fmt.Errorf("failed to save period summary: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetPeriodSummary(periodKey string) (*PeriodSummary, error) {
	// Try to select with analysis column first, fallback to without if column doesn't exist
	query := `
	SELECT period_key, period_type, start_time, end_time, screenshots, summary, COALESCE(analysis, '')
	FROM period_summaries
	WHERE period_key = ?
	`
	var summary PeriodSummary
	var startTimeStr, endTimeStr string
	err := s.db.QueryRow(query, periodKey).Scan(
		&summary.PeriodKey, &summary.PeriodType, &startTimeStr, &endTimeStr, &summary.Screenshots, &summary.Summary, &summary.Analysis,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		// Fallback for old schema without analysis column
		queryOld := `
		SELECT period_key, period_type, start_time, end_time, screenshots, summary
		FROM period_summaries
		WHERE period_key = ?
		`
		err = s.db.QueryRow(queryOld, periodKey).Scan(
			&summary.PeriodKey, &summary.PeriodType, &startTimeStr, &endTimeStr, &summary.Screenshots, &summary.Summary,
		)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get period summary: %w", err)
		}
		summary.Analysis = ""
	}
	summary.StartTime, err = time.Parse(time.RFC3339Nano, startTimeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse start_time: %w", err)
	}
	summary.EndTime, err = time.Parse(time.RFC3339Nano, endTimeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse end_time: %w", err)
	}
	return &summary, nil
}

func (s *SQLiteStorage) DeletePeriodSummary(periodKey string) error {
	query := `DELETE FROM period_summaries WHERE period_key = ?`
	_, err := s.db.Exec(query, periodKey)
	return err
}

func (s *SQLiteStorage) QueryPeriodSummaries(periodType string, start, end time.Time) ([]*PeriodSummary, error) {
	query := `
	SELECT period_key, period_type, start_time, end_time, screenshots, summary, COALESCE(analysis, '')
	FROM period_summaries
	WHERE period_type = ? AND start_time >= ? AND end_time <= ?
	ORDER BY start_time ASC
	`
	rows, err := s.db.Query(query, periodType, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("failed to query period summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*PeriodSummary
	for rows.Next() {
		var ps PeriodSummary
		var startTimeStr, endTimeStr string
		if err := rows.Scan(&ps.PeriodKey, &ps.PeriodType, &startTimeStr, &endTimeStr, &ps.Screenshots, &ps.Summary, &ps.Analysis); err != nil {
			return nil, fmt.Errorf("failed to scan period summary: %w", err)
		}
		ps.StartTime, err = time.Parse(time.RFC3339Nano, startTimeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start_time: %w", err)
		}
		ps.EndTime, err = time.Parse(time.RFC3339Nano, endTimeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse end_time: %w", err)
		}
		summaries = append(summaries, &ps)
	}
	return summaries, rows.Err()
}

func (s *SQLiteStorage) CleanupOldRecords(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	deleteScreenshots := `DELETE FROM screenshots WHERE timestamp < ?`
	if _, err := s.db.Exec(deleteScreenshots, cutoff.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("failed to cleanup old screenshots: %w", err)
	}

	deleteSummaries := `DELETE FROM hour_summaries WHERE date < ?`
	if _, err := s.db.Exec(deleteSummaries, cutoff.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("failed to cleanup old summaries: %w", err)
	}

	return nil
}

// DeleteScreenshotsByIDs deletes screenshot records by their IDs
func (s *SQLiteStorage) DeleteScreenshotsByIDs(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`DELETE FROM screenshots WHERE id IN (%s)`, strings.Join(placeholders, ","))
	_, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete screenshots: %w", err)
	}

	return nil
}

// ClearAllSummaries deletes all hour summaries and period summaries
func (s *SQLiteStorage) ClearAllSummaries() error {
	if _, err := s.db.Exec("DELETE FROM hour_summaries"); err != nil {
		return fmt.Errorf("failed to clear hour summaries: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM period_summaries"); err != nil {
		return fmt.Errorf("failed to clear period summaries: %w", err)
	}

	return nil
}

// GetAllScreenshots returns all screenshot records ordered by timestamp
func (s *SQLiteStorage) GetAllScreenshots() ([]*ScreenshotRecord, error) {
	query := `
	SELECT id, timestamp, screen_id, image_path, analysis, hour_key
	FROM screenshots
	ORDER BY timestamp ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all screenshots: %w", err)
	}
	defer rows.Close()

	var records []*ScreenshotRecord
	for rows.Next() {
		var r ScreenshotRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.ScreenID, &r.ImagePath, &r.Analysis, &r.HourKey); err != nil {
			return nil, fmt.Errorf("failed to scan screenshot: %w", err)
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// LockScreenDetector is a function type that checks if a screenshot is a lock screen
// Returns true if the screenshot is a lock screen, false otherwise
type LockScreenDetector func(imagePath string) (bool, error)

// RebuildFromDirectory scans the screenshot directory and rebuilds the database
// If lockScreenDetector is provided, it will be used to filter out lock screen screenshots
func (s *SQLiteStorage) RebuildFromDirectory(storagePath string, lockScreenDetector LockScreenDetector) (int, error) {
	_, err := s.db.Exec("DELETE FROM screenshots")
	if err != nil {
		return 0, fmt.Errorf("failed to clear screenshots table: %w", err)
	}

	_, err = s.db.Exec("DELETE FROM hour_summaries")
	if err != nil {
		return 0, fmt.Errorf("failed to clear hour_summaries table: %w", err)
	}

	_, err = s.db.Exec("DELETE FROM period_summaries")
	if err != nil {
		return 0, fmt.Errorf("failed to clear period_summaries table: %w", err)
	}

	count, err := s.scanAndImportScreenshots(storagePath, lockScreenDetector)
	if err != nil {
		return count, fmt.Errorf("failed to scan and import screenshots: %w", err)
	}

	return count, nil
}

func (s *SQLiteStorage) scanAndImportScreenshots(storagePath string, lockScreenDetector LockScreenDetector) (int, error) {
	var count int
	var skippedLockScreens int
	var records []*ScreenshotRecord

	err := filepath.Walk(storagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(strings.ToLower(path), ".png") {
			return nil
		}

		// Check if this is a lock screen screenshot
		if lockScreenDetector != nil {
			isLockScreen, err := lockScreenDetector(path)
			if err != nil {
				// Log error but continue processing
				fmt.Fprintf(os.Stderr, "WARNING: Failed to check if screenshot is lock screen (%s): %v, proceeding anyway\n", path, err)
			} else if isLockScreen {
				skippedLockScreens++
				fmt.Fprintf(os.Stdout, "Skipping lock screen screenshot: %s\n", path)
				return nil
			}
		}

		relPath, err := filepath.Rel(storagePath, path)
		if err != nil {
			return nil
		}

		record, err := parseScreenshotPath(path, relPath)
		if err != nil {
			return nil
		}

		records = append(records, record)
		return nil
	})

	if err != nil {
		return count, err
	}

	for _, record := range records {
		if err := s.SaveScreenshot(record); err != nil {
			continue
		}
		count++
	}

	if skippedLockScreens > 0 {
		fmt.Fprintf(os.Stdout, "Skipped %d lock screen screenshot(s) during import\n", skippedLockScreens)
	}

	return count, nil
}

func parseScreenshotPath(fullPath, relPath string) (*ScreenshotRecord, error) {
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid path structure: %s", relPath)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid year: %s", parts[0])
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid month: %s", parts[1])
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid day: %s", parts[2])
	}

	hour, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, fmt.Errorf("invalid hour: %s", parts[3])
	}

	filename := parts[4]
	filenameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))
	timeParts := strings.Split(filenameWithoutExt, "-")
	if len(timeParts) != 2 {
		return nil, fmt.Errorf("invalid filename format: %s", filename)
	}

	minute, err := strconv.Atoi(timeParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid minute: %s", timeParts[0])
	}

	second, err := strconv.Atoi(timeParts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid second: %s", timeParts[1])
	}

	timestamp := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)

	record := &ScreenshotRecord{
		ID:        generateID(),
		Timestamp: timestamp,
		ScreenID:  0,
		ImagePath: fullPath,
		Analysis:  "",
	}
	record.GenerateHourKey()

	return record, nil
}
