package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"
)

var scanConfigPath string
var scanDelete bool

func NewScanInvalidReportsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan-invalid-reports",
		Short: "Scan and detect invalid report files",
		Long: `Scan reports directory and detect invalid report files.
Invalid reports include:
- Parse errors (missing required fields, invalid time formats)
- Invalid content (empty or placeholder summaries, failed analysis)
- Path mismatches (period_key from path doesn't match start_time)
- Logic errors (screenshot count 0 but has summary, no summary but has analysis)

Use --delete to automatically delete invalid reports.`,
		RunE: runScanInvalidReports,
	}

	cmd.Flags().StringVarP(&scanConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().BoolVarP(&scanDelete, "delete", "d", false, "Delete invalid reports automatically")

	return cmd
}

func runScanInvalidReports(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(scanConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Storage.ReportsPath == "" {
		return fmt.Errorf("reports path not configured")
	}

	fmt.Printf("Scanning reports directory: %s\n", cfg.Storage.ReportsPath)
	fmt.Println()

	// Detect invalid reports
	issues, err := storage.DetectInvalidReports(cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to scan reports: %w", err)
	}

	if len(issues) == 0 {
		fmt.Println("✓ No invalid reports found!")
		return nil
	}

	// Group issues by category
	issuesByCategory := make(map[string][]storage.InvalidReportIssue)
	for _, issue := range issues {
		issuesByCategory[issue.Category] = append(issuesByCategory[issue.Category], issue)
	}

	// Print results by category
	fmt.Printf("=== Invalid Reports Found: %d ===\n\n", len(issues))

	for category, categoryIssues := range issuesByCategory {
		fmt.Printf("Category: %s (%d issues)\n", category, len(categoryIssues))
		for _, issue := range categoryIssues {
			fmt.Printf("  - %s\n", issue.FilePath)
			fmt.Printf("    Issue: %s\n", issue.Issue)
		}
		fmt.Println()
	}

	// Print summary
	fmt.Println("=== Summary ===")
	for category, categoryIssues := range issuesByCategory {
		fmt.Printf("%s: %d\n", category, len(categoryIssues))
	}
	fmt.Printf("Total invalid reports: %d\n", len(issues))

	// Delete if requested
	if scanDelete {
		fmt.Println()
		fmt.Println("Deleting invalid reports...")

		st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
		if err != nil {
			return fmt.Errorf("failed to create storage: %w", err)
		}
		defer st.Close()

		deletedCount := 0
		failedCount := 0

		// Get unique file paths (a file might have multiple issues)
		filePaths := make(map[string]bool)
		for _, issue := range issues {
			filePaths[issue.FilePath] = true
		}

		for filePath := range filePaths {
			// Extract period key from file path
			// Try to infer period type
			parser := storage.NewReportParser(cfg.Storage.ReportsPath)
			parsed, err := parser.ParsePeriodReport(filePath)
			if err != nil {
				// Can't parse, try to delete file directly
				if err := os.Remove(filePath); err != nil {
					fmt.Printf("  Failed to delete %s: %v\n", filePath, err)
					failedCount++
				} else {
					fmt.Printf("  Deleted: %s\n", filePath)
					deletedCount++
				}
				continue
			}

			periodType := parsed.PeriodType
			if periodType == "" {
				periodType = inferPeriodTypeFromPath(filePath)
			}

			if periodType != "" {
				periodKey, err := storage.ExtractPeriodKeyFromPath(filePath, periodType)
				if err == nil {
					// Delete from database
					if err := st.DeletePeriodSummary(periodKey); err != nil {
						// Log but continue
						fmt.Printf("  Warning: Failed to delete database record for %s: %v\n", periodKey, err)
					}
				}
			}

			// Delete file
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("  Failed to delete %s: %v\n", filePath, err)
				failedCount++
			} else {
				fmt.Printf("  Deleted: %s\n", filePath)
				deletedCount++
			}
		}

		fmt.Println()
		fmt.Printf("Deleted: %d files\n", deletedCount)
		if failedCount > 0 {
			fmt.Printf("Failed: %d files\n", failedCount)
		}
	} else {
		fmt.Println()
		fmt.Println("Use --delete to automatically delete invalid reports")
	}

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

