package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"stuff-time/internal/analyzer"
	"stuff-time/internal/config"
	"stuff-time/internal/evaluator"
	"stuff-time/internal/storage"
)

var evaluateConfigPath string
var evaluatePeriodKey string
var evaluatePeriodType string
var evaluateDate string
var evaluateOutput string

func NewEvaluateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evaluate",
		Short: "Evaluate period report quality using LLM",
		Long:  "Evaluate the quality of a period report (accuracy, relevance, depth) using LLM and generate a Markdown evaluation report.",
		RunE:  runEvaluate,
	}

	cmd.Flags().StringVarP(&evaluateConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().StringVar(&evaluatePeriodKey, "period-key", "", "Directly specify period key (e.g., \"2025-11-19\")")
	cmd.Flags().StringVarP(&evaluatePeriodType, "period-type", "p", "", "Period type (hour, day, week, month, year)")
	cmd.Flags().StringVarP(&evaluateDate, "date", "d", "", "Date for period (YYYY-MM-DD), used with --period-type")
	cmd.Flags().StringVarP(&evaluateOutput, "output", "o", "", "Output path for evaluation report (default: save to reports/evaluations/)")

	return cmd
}

func runEvaluate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(evaluateConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Storage.EnsureDBPath(); err != nil {
		return fmt.Errorf("failed to create db path: %w", err)
	}

	if err := cfg.Storage.EnsureReportsPath(); err != nil {
		return fmt.Errorf("failed to create reports path: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	// Determine period key
	var periodKey string
	if evaluatePeriodKey != "" {
		periodKey = evaluatePeriodKey
	} else if evaluatePeriodType != "" {
		periodKey, err = buildPeriodKey(evaluatePeriodType, evaluateDate)
		if err != nil {
			return fmt.Errorf("failed to build period key: %w", err)
		}
	} else {
		return fmt.Errorf("must specify either --period-key or --period-type")
	}

	// Get period summary from database
	summary, err := st.GetPeriodSummary(periodKey)
	if err != nil {
		return fmt.Errorf("failed to get period summary: %w", err)
	}
	if summary == nil {
		return fmt.Errorf("period summary not found for key: %s", periodKey)
	}

	// Create analyzer
	openAI := analyzer.NewOpenAI(
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
	)

	// Get screenshot records for traceability
	var screenshotRecords map[string]*storage.ScreenshotRecord
	screenshotIDs := strings.Split(summary.Screenshots, ",")
	if len(screenshotIDs) > 0 && summary.Screenshots != "" {
		// Filter out empty IDs
		validIDs := make([]string, 0, len(screenshotIDs))
		for _, id := range screenshotIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				validIDs = append(validIDs, id)
			}
		}
		if len(validIDs) > 0 {
			screenshotRecords, err = st.GetScreenshotsByIDs(validIDs)
			if err != nil {
				return fmt.Errorf("failed to get screenshot records: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Found %d/%d screenshot records for traceability\n", len(screenshotRecords), len(validIDs))
		}
	}

	// Create evaluator
	eval := evaluator.NewEvaluator(
		openAI,
		cfg.Evaluator.EvaluationPromptContent,
		cfg.Evaluator.ReportContentContent,
		cfg.Evaluator.ScreenshotSourceContent,
		cfg.Evaluator.ReportFormatContent,
		cfg.Evaluator.ScreenshotSourceSectionContent,
	)

	// Evaluate report
	fmt.Fprintf(os.Stdout, "Evaluating period report (key: %s)...\n", periodKey)
	evaluationReport, err := eval.EvaluateReport(summary, screenshotRecords)
	if err != nil {
		return fmt.Errorf("failed to evaluate report: %w", err)
	}

	// Determine output path
	outputPath := evaluateOutput
	if outputPath == "" {
		outputPath = buildEvaluationReportPath(cfg.Storage.ReportsPath, summary)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write evaluation report
	if err := os.WriteFile(outputPath, []byte(evaluationReport), 0644); err != nil {
		return fmt.Errorf("failed to write evaluation report: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Evaluation report saved: %s\n", outputPath)
	return nil
}

func buildPeriodKey(periodType string, date string) (string, error) {
	var now time.Time
	var err error

	if date != "" {
		now, err = time.Parse("2006-01-02", date)
		if err != nil {
			return "", fmt.Errorf("invalid date format: %w", err)
		}
	} else {
		now = time.Now()
	}

	var startTime time.Time
	var periodKey string

	switch periodType {
	case "hour":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		periodKey = startTime.Format("2006-01-02-15")
	case "day":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		periodKey = startTime.Format("2006-01-02")
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		startTime = startTime.AddDate(0, 0, -(weekday - 1))
		periodKey = startTime.Format("2006-01-02") + "-week"
	case "month":
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodKey = startTime.Format("2006-01")
	case "year":
		startTime = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		periodKey = startTime.Format("2006")
	default:
		return "", fmt.Errorf("unsupported period type: %s", periodType)
	}

	return periodKey, nil
}

func buildEvaluationReportPath(reportsPath string, summary *storage.PeriodSummary) string {
	periodType := summary.PeriodType
	var evalDir string
	var filename string

	switch periodType {
	case "year":
		yearDir := summary.StartTime.Format("2006")
		evalDir = filepath.Join(reportsPath, yearDir)
		filename = "year-evaluation.md"
	case "month":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir)
		filename = "month-evaluation.md"
	case "week":
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir)
		// 使用Calendar Week（月内周号）
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		filename = fmt.Sprintf("week-W%d-evaluation.md", weekNum)
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
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir)
		// Extract segment index from period key (format: YYYY-MM-DD-segment-N)
		parts := strings.Split(summary.PeriodKey, "-")
		if len(parts) >= 5 && parts[3] == "segment" {
			segmentNum := parts[4]
			filename = fmt.Sprintf("work-segment-%s-evaluation.md", segmentNum)
		} else {
			filename = fmt.Sprintf("%s-evaluation.md", summary.PeriodKey)
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
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir)
		filename = "day-evaluation.md"
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
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir, hourDir)
		filename = "hour-evaluation.md"
	case "halfhour":
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
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir, hourDir)
		minute := summary.StartTime.Format("04")
		filename = fmt.Sprintf("halfhour-%s-evaluation.md", minute)
	default:
		yearDir := summary.StartTime.Format("2006")
		quarter := (int(summary.StartTime.Month())-1)/3 + 1
		quarterDir := fmt.Sprintf("Q%d", quarter)
		monthDir := summary.StartTime.Format("01")
		// 计算Calendar Week
		day := summary.StartTime.Day()
		weekNum := ((day - 1) / 7) + 1
		weekDir := fmt.Sprintf("W%d", weekNum)
		dayDir := summary.StartTime.Format("02")
		evalDir = filepath.Join(reportsPath, yearDir, quarterDir, monthDir, weekDir, dayDir)
		filename = fmt.Sprintf("%s-evaluation.md", periodType)
	}

	return filepath.Join(evalDir, filename)
}
