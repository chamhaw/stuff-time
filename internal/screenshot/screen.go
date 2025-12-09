package screenshot

/*
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <IOKit/IOKitLib.h>
*/
import "C"
import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kbinani/screenshot"
)

func GetMouseScreenID() (int, error) {
	numDisplays := screenshot.NumActiveDisplays()
	if numDisplays == 0 {
		return 0, fmt.Errorf("no active displays")
	}

	if numDisplays == 1 {
		return 0, nil
	}

	mouseX, mouseY := getMousePosition()
	if mouseX == 0 && mouseY == 0 {
		return 0, fmt.Errorf("failed to get mouse position, may need screen recording permission")
	}

	for i := 0; i < numDisplays; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		if mouseX >= bounds.Min.X && mouseX < bounds.Max.X &&
			mouseY >= bounds.Min.Y && mouseY < bounds.Max.Y {
			return i, nil
		}
	}

	return 0, nil
}

func getMousePosition() (int, int) {
	event := C.CGEventCreate(C.CGEventSourceRef(0))
	if event == 0 {
		return 0, 0
	}
	defer C.CFRelease(C.CFTypeRef(event))

	point := C.CGEventGetLocation(event)
	return int(point.x), int(point.y)
}

// IsScreenLocked checks if the macOS screen is currently locked
// Uses multiple methods for reliability:
// 1. Check if loginwindow is the frontmost application (primary method)
// 2. Check screen saver state as fallback
func IsScreenLocked() (bool, error) {
	// Method 1: Check if loginwindow is frontmost (most reliable)
	locked1, err1 := checkLoginWindowFrontmost()
	if err1 == nil && locked1 {
		return true, nil
	}

	// Method 2: Check screen saver state as additional verification
	locked2, err2 := checkScreenSaverActive()
	if err2 == nil && locked2 {
		return true, nil
	}

	// If both methods fail, log the errors but assume not locked
	if err1 != nil && err2 != nil {
		return false, fmt.Errorf("both lock detection methods failed: loginwindow check: %v, screensaver check: %v", err1, err2)
	}

	// If at least one method succeeded, return its result
	if err1 == nil {
		return locked1, nil
	}
	if err2 == nil {
		return locked2, nil
	}

	return false, nil
}

// checkLoginWindowFrontmost checks if loginwindow is the frontmost application
func checkLoginWindowFrontmost() (bool, error) {
	cmd := exec.Command("osascript", "-e", "tell application \"System Events\" to get name of first process whose frontmost is true")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check frontmost process: %w", err)
	}

	processName := strings.TrimSpace(string(output))
	isLocked := strings.ToLower(processName) == "loginwindow"
	return isLocked, nil
}

// checkScreenSaverActive checks if screen saver is active (indicates lock screen)
func checkScreenSaverActive() (bool, error) {
	cmd := exec.Command("osascript", "-e", "tell application \"System Events\" to tell screen saver preferences to get running")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check screen saver state: %w", err)
	}

	result := strings.ToLower(strings.TrimSpace(string(output)))
	return result == "true", nil
}
