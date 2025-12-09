package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
)

var daemonConfigPath string

func NewDaemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage daemon process (start/stop/restart/status)",
	}

	daemonCmd.AddCommand(NewDaemonStartCmd())
	daemonCmd.AddCommand(NewDaemonStopCmd())
	daemonCmd.AddCommand(NewDaemonRestartCmd())
	daemonCmd.AddCommand(NewDaemonStatusCmd())

	return daemonCmd
}

func NewDaemonStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start stuff-time as daemon",
		RunE:  runDaemonStart,
	}
	cmd.Flags().StringVarP(&daemonConfigPath, "config", "c", "", "Path to config file")
	return cmd
}

func NewDaemonStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop stuff-time daemon",
		RunE:  runDaemonStop,
	}
	return cmd
}

func NewDaemonRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart stuff-time daemon",
		RunE:  runDaemonRestart,
	}
	cmd.Flags().StringVarP(&daemonConfigPath, "config", "c", "", "Path to config file")
	return cmd
}

func NewDaemonStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		RunE:  runDaemonStatus,
	}
	return cmd
}

func getPidFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./stuff-time.pid"
	}
	return filepath.Join(homeDir, ".stuff-time.pid")
}

func getLogFile() string {
	cfg, err := config.Load(daemonConfigPath)
	if err != nil {
		workDir, err := os.Getwd()
		if err != nil {
			return "./stuff-time.log"
		}
		return filepath.Join(workDir, "stuff-time.log")
	}

	return cfg.Storage.LogPath
}

func readPid() (int, error) {
	pidFile := getPidFile()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func writePid(pid int) error {
	pidFile := getPidFile()
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

func removePidFile() error {
	pidFile := getPidFile()
	return os.Remove(pidFile)
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	pid, err := readPid()
	if err == nil && isProcessRunning(pid) {
		return fmt.Errorf("daemon is already running (PID: %d)", pid)
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	logFile := getLogFile()
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFileHandle.Close()

	var cmdArgs []string
	cmdArgs = append(cmdArgs, "start")
	if daemonConfigPath != "" {
		cmdArgs = append(cmdArgs, "--config", daemonConfigPath)
	}

	processCmd := exec.Command(executable, cmdArgs...)
	processCmd.Stdout = logFileHandle
	processCmd.Stderr = logFileHandle
	processCmd.Dir, _ = os.Getwd()

	if err := processCmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	if err := writePid(processCmd.Process.Pid); err != nil {
		processCmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Printf("Daemon started (PID: %d, Log: %s)\n", processCmd.Process.Pid, logFile)
	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	pid, err := readPid()
	if err != nil {
		return fmt.Errorf("daemon is not running (PID file not found)")
	}

	if !isProcessRunning(pid) {
		removePidFile()
		return fmt.Errorf("daemon is not running (process not found)")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		removePidFile()
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isProcessRunning(pid) {
			removePidFile()
			fmt.Printf("Daemon stopped (PID: %d)\n", pid)
			return nil
		}
	}

	process.Signal(syscall.SIGKILL)
	time.Sleep(500 * time.Millisecond)
	removePidFile()
	fmt.Printf("Daemon force stopped (PID: %d)\n", pid)
	return nil
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	if err := runDaemonStop(cmd, args); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
	time.Sleep(1 * time.Second)
	return runDaemonStart(cmd, args)
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	pid, err := readPid()
	if err != nil {
		fmt.Println("Status: Not running")
		return nil
	}

	if isProcessRunning(pid) {
		fmt.Printf("Status: Running (PID: %d)\n", pid)
		fmt.Printf("PID file: %s\n", getPidFile())
		fmt.Printf("Log file: %s\n", getLogFile())
	} else {
		fmt.Println("Status: Not running (stale PID file)")
		removePidFile()
	}
	return nil
}
