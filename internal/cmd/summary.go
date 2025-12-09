package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"
)

var summaryConfigPath string
var summaryPeriod string
var summaryDate string

func NewSummaryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "View cumulative summaries by day/week/month/year",
		RunE:  runSummary,
	}
	cmd.Flags().StringVarP(&summaryPeriod, "period", "p", "day", "Summary period: day, week, month, year")
	cmd.Flags().StringVarP(&summaryDate, "date", "d", "", "Date for summary (YYYY-MM-DD), defaults to today")
	cmd.Flags().StringVarP(&summaryConfigPath, "config", "c", "", "Path to config file")
	return cmd
}

func runSummary(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(summaryConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	var start, end time.Time
	var periodType string

	now := time.Now()
	if summaryDate != "" {
		date, err := time.Parse("2006-01-02", summaryDate)
		if err != nil {
			return fmt.Errorf("invalid date format: %w", err)
		}
		now = date
	}

	switch summaryPeriod {
	case "day":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1)
		periodType = "day"
		fmt.Fprintf(os.Stdout, "Daily Summary for %s\n", start.Format("2006-01-02"))
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		start = start.AddDate(0, 0, -(weekday - 1))
		end = start.AddDate(0, 0, 7)
		periodType = "week"
		fmt.Fprintf(os.Stdout, "Weekly Summary for week starting %s\n", start.Format("2006-01-02"))
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0)
		periodType = "month"
		fmt.Fprintf(os.Stdout, "Monthly Summary for %s\n", start.Format("2006-01"))
	case "year":
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(1, 0, 0)
		periodType = "year"
		fmt.Fprintf(os.Stdout, "Yearly Summary for %d\n", now.Year())
	default:
		return fmt.Errorf("invalid period: %s (must be day, week, month, or year)", summaryPeriod)
	}

	fmt.Fprintf(os.Stdout, "================\n\n")

	// First try to get period summary for the specific period
	summaries, err := st.QueryPeriodSummaries(periodType, start, end)
	if err != nil {
		return fmt.Errorf("failed to query period summaries: %w", err)
	}

	if len(summaries) > 0 {
		// Found period summary, display it
		fmt.Fprintf(os.Stdout, "Period Summary:\n\n")
		for _, s := range summaries {
			fmt.Fprintf(os.Stdout, "%s (%s - %s):\n%s\n\n",
				s.PeriodKey,
				s.StartTime.Format("2006-01-02 15:04"),
				s.EndTime.Format("2006-01-02 15:04"),
				s.Summary)
		}
	} else {
		// Fallback to hour summaries if period summary not found
		hourSummaries, err := st.QueryHourSummariesByDateRange(start, end)
		if err != nil {
			return fmt.Errorf("failed to query hour summaries: %w", err)
		}

		if len(hourSummaries) == 0 {
			fmt.Fprintf(os.Stdout, "No data found for the specified period.\n")
			return nil
		}

		fmt.Fprintf(os.Stdout, "Hour Summaries (period summary not available):\n\n")
		for _, s := range hourSummaries {
			fmt.Fprintf(os.Stdout, "%s:\n%s\n\n", s.HourKey, s.Summary)
		}
	}

	return nil
}
