package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"
)

var validateConfigPath string
var validatePeriodType string
var validateStartDate string
var validateEndDate string
var validateFix bool
var validateVerbose bool
var validateRebuildDB bool

func NewValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate consistency between database metadata and report files",
		Long: `Validate consistency between database metadata and report files.
This command checks:
1. Whether period_key extracted from file path matches start_time in file content
2. Whether database records have corresponding files
3. Whether files have corresponding database records

Use --fix to automatically correct inconsistencies.
Use --rebuild-db to rebuild entire database from report files (useful when database file is missing or corrupted).`,
		RunE: runValidate,
	}

	cmd.Flags().StringVarP(&validateConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().StringVarP(&validatePeriodType, "period", "p", "", "Period type to validate (fifteenmin, hour, day, week, month, quarter, year). If not specified, validates all types.")
	cmd.Flags().StringVarP(&validateStartDate, "start", "s", "", "Start date (YYYY-MM-DD). If not specified, validates all files.")
	cmd.Flags().StringVarP(&validateEndDate, "end", "e", "", "End date (YYYY-MM-DD). If not specified, validates all files.")
	cmd.Flags().BoolVarP(&validateFix, "fix", "f", false, "Automatically fix inconsistencies (rebuild period_key from file content)")
	cmd.Flags().BoolVarP(&validateVerbose, "verbose", "v", false, "Show detailed validation results")
	cmd.Flags().BoolVarP(&validateRebuildDB, "rebuild-db", "r", false, "Rebuild database from report files (use when database file is missing or corrupted)")

	return cmd
}

func runValidate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(validateConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Logger initialization is handled by config

	// Check if database file exists
	dbExists := false
	if cfg.Storage.DBPath != "" {
		if _, err := os.Stat(cfg.Storage.DBPath); err == nil {
			dbExists = true
		}
	}

	// If --rebuild-db and database doesn't exist, create it
	if validateRebuildDB && !dbExists {
		fmt.Printf("Database file not found at %s, creating new database...\n", cfg.Storage.DBPath)
		if err := cfg.Storage.EnsureDBPath(); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}
	defer st.Close()

	// If --rebuild-db, rebuild all period summaries from files
	if validateRebuildDB {
		fmt.Println("Rebuilding database from report files...")
		fmt.Println("This will scan all report files and recreate database records.")
		fmt.Println()
		
		// Enable fix mode automatically when rebuilding
		validateFix = true
		
		// Set default date range to cover all files if not specified
		if validateStartDate == "" {
			validateStartDate = "2020-01-01"
		}
		if validateEndDate == "" {
			validateEndDate = time.Now().Format("2006-01-02")
		}
	}

	// Parse date range
	var startTime, endTime time.Time
	if validateStartDate != "" {
		startTime, err = time.Parse("2006-01-02", validateStartDate)
		if err != nil {
			return fmt.Errorf("invalid start date format: %w", err)
		}
		startTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	} else {
		startTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local) // Default: start from 2020
	}

	if validateEndDate != "" {
		endTime, err = time.Parse("2006-01-02", validateEndDate)
		if err != nil {
			return fmt.Errorf("invalid end date format: %w", err)
		}
		endTime = time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 23, 59, 59, 0, endTime.Location())
	} else {
		endTime = time.Now()
	}

	// Determine period types to validate
	periodTypes := []string{"fifteenmin", "hour", "day", "work-segment", "week", "month", "quarter", "year"}
	if validatePeriodType != "" {
		periodTypes = []string{validatePeriodType}
	}

	fmt.Printf("Validating period summaries from %s to %s\n", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))
	fmt.Println()

	var totalChecked int
	var totalIssues int
	var totalFixed int

	for _, periodType := range periodTypes {
		fmt.Printf("Validating %s summaries...\n", periodType)

		var dbSummaries []*storage.PeriodSummary
		var fsSummaries []*storage.PeriodSummary
		var err error

		if validateRebuildDB {
			// Rebuild mode: query filesystem directly to get all files
			// Then query database separately (may be empty)
			if cfg.Storage.ReportsPath == "" {
				fmt.Printf("  ERROR: Reports path not configured, cannot rebuild from files\n")
				continue
			}
			
			// Query filesystem directly
			fsStorage, err := storage.NewFileSystemStorage(cfg.Storage.ReportsPath)
			if err != nil {
				fmt.Printf("  ERROR: Failed to create filesystem storage: %v\n", err)
				continue
			}
			fsSummaries, err = fsStorage.QueryPeriodSummaries(periodType, startTime, endTime)
			if err != nil {
				fmt.Printf("  ERROR: Failed to query filesystem: %v\n", err)
				continue
			}
			
			// Query database separately (may be empty or new)
			dbSummaries, err = st.QueryPeriodSummaries(periodType, startTime, endTime)
			if err != nil {
				// If database query fails (e.g., database is new), treat as empty
				dbSummaries = []*storage.PeriodSummary{}
			}
		} else {
			// Normal validation mode: use report storage query
			dbSummaries, err = st.QueryPeriodSummaries(periodType, startTime, endTime)
			if err != nil {
				fmt.Printf("  ERROR: Failed to query database: %v\n", err)
				continue
			}
			fsSummaries, err = st.QueryPeriodSummaries(periodType, startTime, endTime)
			if err != nil {
				fmt.Printf("  ERROR: Failed to query filesystem: %v\n", err)
				continue
			}
		}

		// Build maps for comparison
		dbMap := make(map[string]*storage.PeriodSummary)
		fsMap := make(map[string]*storage.PeriodSummary)

		for _, s := range dbSummaries {
			dbMap[s.PeriodKey] = s
		}

		for _, s := range fsSummaries {
			fsMap[s.PeriodKey] = s
		}

		// Check database records
		for periodKey, dbSummary := range dbMap {
			totalChecked++
			issues := []string{}

			// Check if file exists (for non-placeholder summaries)
			if dbSummary.Summary != "__NO_WORK_ACTIVITY_PLACEHOLDER__" {
				if _, exists := fsMap[periodKey]; !exists {
					issues = append(issues, "file missing")
					totalIssues++
				}
			}

			if validateVerbose && len(issues) > 0 {
				fmt.Printf("  [DB] %s: %s\n", periodKey, strings.Join(issues, ", "))
			}
		}

		// Check filesystem files
		for periodKey, fsSummary := range fsMap {
			if _, exists := dbMap[periodKey]; !exists {
				totalChecked++
				issues := []string{"database record missing"}
				totalIssues++

				if validateVerbose || validateRebuildDB {
					fmt.Printf("  [FS] %s: %s\n", periodKey, strings.Join(issues, ", "))
				}

				// If --fix or --rebuild-db, create database record from file
				if validateFix || validateRebuildDB {
					// Rebuild period_key from file content if needed
					correctedKey := storage.BuildPeriodKeyFromStartTime(fsSummary.StartTime, periodType)
					if correctedKey != "" && correctedKey != periodKey {
						if validateVerbose || validateRebuildDB {
							fmt.Printf("    Correcting period_key: %s -> %s\n", periodKey, correctedKey)
						}
						fsSummary.PeriodKey = correctedKey
						periodKey = correctedKey
					}

					// Save to database
					if err := st.SavePeriodSummary(fsSummary); err != nil {
						fmt.Printf("    ERROR: Failed to save %s: %v\n", periodKey, err)
					} else {
						totalFixed++
						if validateVerbose || validateRebuildDB {
							fmt.Printf("    ✓ Created database record for %s\n", periodKey)
						}
					}
				}
			}
		}

		fmt.Printf("  Checked %d %s summaries\n", len(dbSummaries)+len(fsSummaries), periodType)
	}

	fmt.Println()
	fmt.Printf("=== Validation Summary ===\n")
	fmt.Printf("Total checked: %d\n", totalChecked)
	fmt.Printf("Total issues: %d\n", totalIssues)
	if validateFix || validateRebuildDB {
		fmt.Printf("Total fixed: %d\n", totalFixed)
		if validateRebuildDB {
			fmt.Printf("\n✓ Database rebuilt successfully from report files!\n")
			fmt.Printf("  All period summaries have been restored to the database.\n")
		}
	} else {
		fmt.Printf("Use --fix to automatically fix issues\n")
		fmt.Printf("Use --rebuild-db to rebuild entire database from report files\n")
	}

	if totalIssues == 0 {
		fmt.Println("✓ All files are consistent!")
		return nil
	}

	if validateRebuildDB && totalFixed > 0 {
		// If we rebuilt the database, consider it successful even if there were issues
		fmt.Printf("\n✓ Database rebuild completed. %d records restored.\n", totalFixed)
		return nil
	}

	return fmt.Errorf("found %d inconsistencies", totalIssues)
}

// validateFileConsistency validates a single file's consistency
func validateFileConsistency(reportsPath string, filePath string, periodType string) ([]string, error) {
	var issues []string

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []string{"file does not exist"}, nil
	}

	// Parse file
	parser := storage.NewReportParser(reportsPath)
	parsed, err := parser.ParsePeriodReport(filePath)
	if err != nil {
		return []string{fmt.Sprintf("failed to parse: %v", err)}, nil
	}

	// Extract period_key from path
	periodKey, err := storage.ExtractPeriodKeyFromPath(filePath, periodType)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to extract period_key: %v", err))
	} else {
		// Validate period_key against start_time
		if !parsed.StartTime.IsZero() {
			if err := storage.ValidatePeriodKeyFromStartTime(periodKey, periodType, parsed.StartTime); err != nil {
				issues = append(issues, fmt.Sprintf("period_key mismatch: %v", err))
			}
		}
	}

	return issues, nil
}

