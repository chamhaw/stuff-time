package screenshot

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/kbinani/screenshot"
)

func CaptureScreen(screenID int, storagePath string, imageFormat string) (string, error) {
	bounds := screenshot.GetDisplayBounds(screenID)
	
	// Increase timeout to 15 seconds to handle system load variations
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	startTime := time.Now()
	done := make(chan error, 1)
	var img image.Image
	
	go func() {
		var err error
		img, err = screenshot.CaptureRect(bounds)
		done <- err
	}()
	
	select {
	case err := <-done:
		elapsed := time.Since(startTime)
		if err != nil {
			return "", fmt.Errorf("failed to capture screen %d (took %v, bounds: %v): %w", screenID, elapsed, bounds, err)
		}
		// Success - capture completed
	case <-ctx.Done():
		elapsed := time.Since(startTime)
		// More generic error message since this could be various issues
		return "", fmt.Errorf("screenshot capture timeout after %v (15s limit) for screen %d (bounds: %v). This could be due to system load, display issues, or permission problems. Check System Settings > Privacy & Security > Screen Recording if permissions were recently changed", elapsed, screenID, bounds)
	}

	now := time.Now()
	yearDir := now.Format("2006")
	monthDir := now.Format("01")
	dayDir := now.Format("02")
	hourDir := now.Format("15")
	
	// Calculate quarter: Q1-Q4
	month := int(now.Month())
	quarter := (month-1)/3 + 1
	quarterDir := fmt.Sprintf("Q%d", quarter)
	
	// Calculate Calendar Week: W1-W5 (month-based week number)
	day := now.Day()
	weekNum := ((day - 1) / 7) + 1
	weekDir := fmt.Sprintf("W%d", weekNum)

	// Build path: YYYY/QN/MM/WN/DD/HH/
	dir := filepath.Join(storagePath, yearDir, quarterDir, monthDir, weekDir, dayDir, hourDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Filename only contains minute, since parent directory already has year/month/day/hour
	filename := fmt.Sprintf("%s.%s", now.Format("04"), imageFormat)
	filepath := filepath.Join(dir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return "", fmt.Errorf("failed to encode image: %w", err)
	}

	return filepath, nil
}
