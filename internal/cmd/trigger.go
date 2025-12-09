package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"
	"stuff-time/internal/task"
)

var triggerConfigPath string
var triggerScreenshot bool
var triggerAnalyze bool
var triggerVerbose bool

func NewTriggerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Debug: Manually trigger screenshot capture or analysis",
		Long:  "Debug command for manually triggering operations. Use for testing and debugging purposes only.",
		RunE:  runTrigger,
	}

	cmd.Flags().StringVarP(&triggerConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().BoolVar(&triggerScreenshot, "screenshot", false, "Trigger screenshot capture")
	cmd.Flags().BoolVar(&triggerAnalyze, "analyze", false, "Trigger batch analysis")
	cmd.Flags().BoolVarP(&triggerVerbose, "verbose", "v", false, "Enable verbose output for debugging")
	cmd.Flags().BoolP("all", "a", false, "Trigger all debug operations (screenshot, analyze)")

	return cmd
}

func runTrigger(cmd *cobra.Command, args []string) error {
	if triggerVerbose {
		fmt.Fprintf(os.Stdout, "[VERBOSE] Loading configuration...\n")
	}
	
	cfg, err := config.Load(triggerConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if triggerVerbose {
		fmt.Fprintf(os.Stdout, "[VERBOSE] Config loaded: storage_path=%s, db_path=%s\n", 
			cfg.Screenshot.StoragePath, cfg.Storage.DBPath)
	}

	if triggerVerbose {
		fmt.Fprintf(os.Stdout, "[VERBOSE] Ensuring storage path...\n")
	}
	if err := cfg.Screenshot.EnsureStoragePath(); err != nil {
		return fmt.Errorf("failed to create storage path: %w", err)
	}

	if triggerVerbose {
		fmt.Fprintf(os.Stdout, "[VERBOSE] Ensuring DB path...\n")
	}
	if err := cfg.Storage.EnsureDBPath(); err != nil {
		return fmt.Errorf("failed to create db path: %w", err)
	}

	if triggerVerbose {
		fmt.Fprintf(os.Stdout, "[VERBOSE] Initializing storage...\n")
	}
	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	if triggerVerbose {
		fmt.Fprintf(os.Stdout, "[VERBOSE] Creating executor...\n")
	}
	executor, err := task.NewExecutor(cfg, st)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	triggerAll, _ := cmd.Flags().GetBool("all")
	if triggerAll {
		triggerScreenshot = true
		triggerAnalyze = true
	}

	if !triggerScreenshot && !triggerAnalyze {
		return fmt.Errorf("please specify at least one operation: --screenshot, --analyze, or --all")
	}

	if triggerScreenshot {
		fmt.Fprintf(os.Stdout, "Triggering screenshot capture...\n")
		if triggerVerbose {
			fmt.Fprintf(os.Stdout, "[VERBOSE] Getting mouse screen ID...\n")
		}
		
		if err := executor.CaptureScreenshot(); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to capture screenshot: %v\n", err)
			if triggerVerbose {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Error details: %+v\n", err)
			}
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "This is likely a macOS permission issue. Please:\n")
			fmt.Fprintf(os.Stderr, "1. Go to System Settings > Privacy & Security > Screen Recording\n")
			fmt.Fprintf(os.Stderr, "2. Enable permission for Terminal (or the app running stuff-time)\n")
			fmt.Fprintf(os.Stderr, "3. Restart the terminal/app after granting permission\n")
			fmt.Fprintf(os.Stderr, "\n")
			
			if !triggerAnalyze {
				return fmt.Errorf("screenshot capture failed")
			}
			fmt.Fprintf(os.Stdout, "Continuing with other operations...\n\n")
		} else {
			fmt.Fprintf(os.Stdout, "Screenshot captured successfully.\n\n")
		}
	}

	if triggerAnalyze {
		fmt.Fprintf(os.Stdout, "Triggering batch analysis...\n")
		if triggerVerbose {
			fmt.Fprintf(os.Stdout, "[VERBOSE] Querying unanalyzed screenshots...\n")
		}
		if err := executor.BatchAnalyze(); err != nil {
			if triggerVerbose {
				fmt.Fprintf(os.Stderr, "[VERBOSE] Analysis error details: %+v\n", err)
			}
			return fmt.Errorf("failed to analyze: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Batch analysis completed successfully.\n\n")
	}

	completed := []string{}
	if triggerScreenshot {
		completed = append(completed, "screenshot")
	}
	if triggerAnalyze {
		completed = append(completed, "analysis")
	}
	
	if len(completed) > 0 {
		fmt.Fprintf(os.Stdout, "Completed debug operations: %v\n", completed)
	}
	return nil
}

