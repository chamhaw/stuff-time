package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"stuff-time/internal/analyzer"
	"stuff-time/internal/config"
	"stuff-time/internal/evaluator"
	"stuff-time/internal/storage"
	"stuff-time/internal/task"
)

var improveConfigPath string
var improvePeriodKey string
var improvePeriodType string
var improveDate string
var improveEvaluationFile string

func NewImproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "improve",
		Short: "Improve period report based on evaluation feedback",
		Long:  "Read evaluation report and generate improved period report based on evaluation feedback. Updates the period summary in database and regenerates report file.",
		RunE:  runImprove,
	}

	cmd.Flags().StringVarP(&improveConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().StringVar(&improvePeriodKey, "period-key", "", "Directly specify period key (e.g., \"2025-11-21\")")
	cmd.Flags().StringVarP(&improvePeriodType, "period-type", "p", "", "Period type (hour, day, week, month, year)")
	cmd.Flags().StringVarP(&improveDate, "date", "d", "", "Date for period (YYYY-MM-DD), used with --period-type")
	cmd.Flags().StringVarP(&improveEvaluationFile, "evaluation-file", "e", "", "Path to evaluation report file (default: auto-detect from standard path)")

	return cmd
}

func runImprove(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(improveConfigPath)
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
	if improvePeriodKey != "" {
		periodKey = improvePeriodKey
	} else if improvePeriodType != "" {
		periodKey, err = buildPeriodKey(improvePeriodType, improveDate)
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

	// Determine evaluation report path
	evaluationPath := improveEvaluationFile
	if evaluationPath == "" {
		evaluationPath = buildEvaluationReportPath(cfg.Storage.ReportsPath, summary)
	}

	// Check if evaluation report exists
	if _, err := os.Stat(evaluationPath); os.IsNotExist(err) {
		return fmt.Errorf("evaluation report not found: %s (use --evaluation-file to specify path)", evaluationPath)
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

	// Get screenshot records for context
	var screenshotRecords map[string]*storage.ScreenshotRecord
	screenshotIDs := strings.Split(summary.Screenshots, ",")
	if len(screenshotIDs) > 0 && summary.Screenshots != "" {
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
			fmt.Fprintf(os.Stdout, "Found %d/%d screenshot records for context\n", len(screenshotRecords), len(validIDs))
		}
	}

	// Create evaluator with improvement prompt
	if cfg.Evaluator.ImprovementPromptContent == "" {
		return fmt.Errorf("improvement prompt not configured (check evaluator.improvement_path in config)")
	}

	eval, err := evaluator.NewEvaluatorWithImprovement(
		openAI,
		cfg.Evaluator.EvaluationPromptContent,
		cfg.Evaluator.ReportContentContent,
		cfg.Evaluator.ScreenshotSourceContent,
		cfg.Evaluator.ReportFormatContent,
		cfg.Evaluator.ScreenshotSourceSectionContent,
		cfg.Evaluator.ImprovementPromptContent,
		cfg.Evaluator.ImprovementScreenshotSourceContent,
	)
	if err != nil {
		return fmt.Errorf("failed to create evaluator: %w", err)
	}

	// Improve report
	fmt.Fprintf(os.Stdout, "Improving period report (key: %s) based on evaluation: %s\n", periodKey, evaluationPath)
	improved, err := eval.ImproveReport(summary, evaluationPath, screenshotRecords)
	if err != nil {
		return fmt.Errorf("failed to improve report: %w", err)
	}

	// Update period summary in database
	summary.Summary = improved.Summary
	summary.Analysis = improved.Analysis

	if err := st.SavePeriodSummary(summary); err != nil {
		return fmt.Errorf("failed to save improved summary: %w", err)
	}

	// Regenerate report file
	executor, err := task.NewExecutor(cfg, st)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Save period summary report
	if err := executor.SavePeriodSummaryReport(summary); err != nil {
		return fmt.Errorf("failed to regenerate report file: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Improved report saved successfully.\n")
	if improved.ImprovementNotes != "" {
		fmt.Fprintf(os.Stdout, "\nImprovement notes:\n%s\n", improved.ImprovementNotes)
	}

	return nil
}
