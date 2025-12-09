package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"stuff-time/internal/config"
)

func TestStorageManager_SaveScreenshot(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	testData := []byte("test screenshot data")

	path, err := sm.SaveScreenshot(testTime, testData)
	if err != nil {
		t.Fatalf("SaveScreenshot failed: %v", err)
	}

	// 验证路径格式
	expectedPath := "2025/Q1/01/W3/15/WS2/10/S3/30.png"
	if path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, path)
	}

	// 验证文件存在
	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("File does not exist: %s", fullPath)
	}

	// 验证文件内容
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("File content mismatch")
	}
}

func TestStorageManager_SaveReport(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	testContent := "# Test Report\n\nThis is a test report."

	path, err := sm.SaveReport(testTime, testContent)
	if err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	// 验证路径格式
	expectedPath := "2025/Q1/01/W3/15/WS2/10/S3/30.md"
	if path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, path)
	}

	// 验证文件存在和内容
	fullPath := filepath.Join(tmpDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("File content mismatch")
	}
}

func TestStorageManager_SaveSummary(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name         string
		level        SummaryLevel
		expectedPath string
	}{
		{
			name:         "分段汇总",
			level:        SummaryLevelSegment,
			expectedPath: "2025/Q1/01/W3/15/WS2/10/S3/summary.md",
		},
		{
			name:         "小时汇总",
			level:        SummaryLevelHour,
			expectedPath: "2025/Q1/01/W3/15/WS2/10/hour.md",
		},
		{
			name:         "工作段汇总",
			level:        SummaryLevelWorkSegment,
			expectedPath: "2025/Q1/01/W3/15/WS2/work-segment.md",
		},
		{
			name:         "天汇总",
			level:        SummaryLevelDay,
			expectedPath: "2025/Q1/01/W3/15/day.md",
		},
		{
			name:         "周汇总",
			level:        SummaryLevelWeek,
			expectedPath: "2025/Q1/01/W3/week.md",
		},
		{
			name:         "月汇总",
			level:        SummaryLevelMonth,
			expectedPath: "2025/Q1/01/month.md",
		},
		{
			name:         "季度汇总",
			level:        SummaryLevelQuarter,
			expectedPath: "2025/Q1/quarter.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testContent := "# Test Summary\n\nThis is a test summary."

			path, err := sm.SaveSummary(testTime, tt.level, testContent)
			if err != nil {
				t.Fatalf("SaveSummary failed: %v", err)
			}

			if path != tt.expectedPath {
				t.Errorf("Expected path %s, got %s", tt.expectedPath, path)
			}

			// 验证文件存在和内容
			fullPath := filepath.Join(tmpDir, path)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}
			if string(content) != testContent {
				t.Errorf("File content mismatch")
			}
		})
	}
}

func TestStorageManager_LegacyFormat(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: false, // 禁用嵌套结构
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	testData := []byte("test data")

	path, err := sm.SaveScreenshot(testTime, testData)
	if err != nil {
		t.Fatalf("SaveScreenshot failed: %v", err)
	}

	// 验证使用旧格式：YYYY/MM/DD/HH/MIN-SEC.png
	expectedPath := "2025/01/15/10/30-45.png"
	if path != expectedPath {
		t.Errorf("Expected legacy path %s, got %s", expectedPath, path)
	}

	// 验证文件存在
	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("File does not exist: %s", fullPath)
	}
}

func TestStorageManager_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	testData := []byte("test data")

	_, err := sm.SaveScreenshot(testTime, testData)
	if err != nil {
		t.Fatalf("SaveScreenshot failed: %v", err)
	}

	// 验证所有必需的目录都被创建
	expectedDirs := []string{
		"2025",
		"2025/Q1",
		"2025/Q1/01",
		"2025/Q1/01/W3",
		"2025/Q1/01/W3/15",
		"2025/Q1/01/W3/15/WS2",
		"2025/Q1/01/W3/15/WS2/10",
		"2025/Q1/01/W3/15/WS2/10/S3",
	}

	for _, dir := range expectedDirs {
		fullPath := filepath.Join(tmpDir, dir)
		info, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			t.Errorf("Directory does not exist: %s", dir)
		} else if err != nil {
			t.Errorf("Error checking directory %s: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("Path is not a directory: %s", dir)
		}
	}
}

func TestStorageManager_SkipLevels(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          1, // 跳过分段层级
		DayWorkSegments:       0, // 跳过工作段层级
		MonthWeeks:            "calendar",
		YearQuarters:          1, // 跳过季度层级
		EnableNestedStructure: true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	testData := []byte("test data")

	path, err := sm.SaveScreenshot(testTime, testData)
	if err != nil {
		t.Fatalf("SaveScreenshot failed: %v", err)
	}

	// 验证路径跳过了季度、工作段和分段层级
	expectedPath := "2025/01/W3/15/10/30.png"
	if path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, path)
	}
}

func TestStorageManager_GetFile_NewFormat(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
		BackwardCompatible:    true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	testData := []byte("test data")

	// 保存文件
	_, err := sm.SaveScreenshot(testTime, testData)
	if err != nil {
		t.Fatalf("SaveScreenshot failed: %v", err)
	}

	// 获取文件
	path, err := sm.GetFile(testTime, FileTypeScreenshot)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("File does not exist: %s", path)
	}

	// 验证文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("File content mismatch")
	}
}

func TestStorageManager_GetFile_LegacyFormat(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
		BackwardCompatible:    true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	testData := []byte("legacy test data")

	// 手动创建旧格式文件
	legacyDir := filepath.Join(tmpDir, "2025", "01", "15", "10")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("Failed to create legacy directory: %v", err)
	}

	legacyFile := filepath.Join(legacyDir, "30-45.png")
	if err := os.WriteFile(legacyFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write legacy file: %v", err)
	}

	// 获取文件（应该找到旧格式文件并自动规范化）
	path, err := sm.GetFile(testTime, FileTypeScreenshot)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}

	// 验证返回的是新格式路径（因为自动规范化）
	expectedNewPath := filepath.Join(tmpDir, "2025", "Q1", "01", "W3", "15", "WS2", "10", "S3", "30.png")
	if path != expectedNewPath {
		t.Errorf("Expected normalized file %s, got %s", expectedNewPath, path)
	}

	// 验证新文件存在且内容正确
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("File content mismatch")
	}

	// 验证旧文件已被删除
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Errorf("Legacy file should have been deleted: %s", legacyFile)
	}
}

func TestStorageManager_GetFile_PreferNewFormat(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
		BackwardCompatible:    true,
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	// 创建新格式文件
	newData := []byte("new format data")
	_, err := sm.SaveScreenshot(testTime, newData)
	if err != nil {
		t.Fatalf("SaveScreenshot failed: %v", err)
	}

	// 创建旧格式文件
	legacyDir := filepath.Join(tmpDir, "2025", "01", "15", "10")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("Failed to create legacy directory: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "30-00.png")
	legacyData := []byte("legacy format data")
	if err := os.WriteFile(legacyFile, legacyData, 0644); err != nil {
		t.Fatalf("Failed to write legacy file: %v", err)
	}

	// 获取文件（应该优先返回新格式）
	path, err := sm.GetFile(testTime, FileTypeScreenshot)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}

	// 验证返回的是新格式文件
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(newData) {
		t.Errorf("Expected new format data, got legacy data")
	}
}

func TestStorageManager_GetFile_NoBackwardCompatible(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.StorageConfig{
		HourSegments:          4,
		DayWorkSegments:       3,
		MonthWeeks:            "calendar",
		YearQuarters:          4,
		EnableNestedStructure: true,
		BackwardCompatible:    false, // 禁用向后兼容
	}

	sm := NewStorageManager(cfg, tmpDir)
	testTime := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)

	// 只创建旧格式文件
	legacyDir := filepath.Join(tmpDir, "2025", "01", "15", "10")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("Failed to create legacy directory: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "30-45.png")
	if err := os.WriteFile(legacyFile, []byte("legacy data"), 0644); err != nil {
		t.Fatalf("Failed to write legacy file: %v", err)
	}

	// 获取文件（应该失败，因为禁用了向后兼容）
	_, err := sm.GetFile(testTime, FileTypeScreenshot)
	if err == nil {
		t.Errorf("Expected GetFile to fail when backward compatible is disabled")
	}
}

func TestIsLegacyFormat(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{
			name:     "旧格式 PNG",
			filename: "30-45.png",
			want:     true,
		},
		{
			name:     "旧格式 MD",
			filename: "30-45.md",
			want:     true,
		},
		{
			name:     "新格式 PNG",
			filename: "30.png",
			want:     false,
		},
		{
			name:     "新格式 MD",
			filename: "30.md",
			want:     false,
		},
		{
			name:     "汇总文件",
			filename: "hour.md",
			want:     false,
		},
		{
			name:     "无效格式",
			filename: "invalid.txt",
			want:     false,
		},
		{
			name:     "空字符串",
			filename: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLegacyFormat(tt.filename)
			if got != tt.want {
				t.Errorf("IsLegacyFormat(%s) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

