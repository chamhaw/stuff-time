package storage

import (
	"testing"
	"time"

	"stuff-time/internal/config"
)

func TestPathCalculator_CalculateHourSegment(t *testing.T) {
	tests := []struct {
		name         string
		hourSegments int
		minute       int
		want         int
	}{
		{
			name:         "4段配置 - 第1段（0-14分钟）",
			hourSegments: 4,
			minute:       0,
			want:         1,
		},
		{
			name:         "4段配置 - 第1段边界",
			hourSegments: 4,
			minute:       14,
			want:         1,
		},
		{
			name:         "4段配置 - 第2段（15-29分钟）",
			hourSegments: 4,
			minute:       15,
			want:         2,
		},
		{
			name:         "4段配置 - 第3段（30-44分钟）",
			hourSegments: 4,
			minute:       30,
			want:         3,
		},
		{
			name:         "4段配置 - 第4段（45-59分钟）",
			hourSegments: 4,
			minute:       45,
			want:         4,
		},
		{
			name:         "4段配置 - 最后一分钟",
			hourSegments: 4,
			minute:       59,
			want:         4,
		},
		{
			name:         "2段配置 - 第1段",
			hourSegments: 2,
			minute:       0,
			want:         1,
		},
		{
			name:         "2段配置 - 第2段",
			hourSegments: 2,
			minute:       30,
			want:         2,
		},
		{
			name:         "6段配置 - 第1段",
			hourSegments: 6,
			minute:       0,
			want:         1,
		},
		{
			name:         "6段配置 - 第2段",
			hourSegments: 6,
			minute:       10,
			want:         2,
		},
		{
			name:         "6段配置 - 第6段",
			hourSegments: 6,
			minute:       50,
			want:         6,
		},
		{
			name:         "1段配置",
			hourSegments: 1,
			minute:       30,
			want:         1,
		},
		{
			name:         "边界测试 - 负数分钟",
			hourSegments: 4,
			minute:       -1,
			want:         1,
		},
		{
			name:         "边界测试 - 超过60分钟",
			hourSegments: 4,
			minute:       65,
			want:         4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StorageConfig{
				HourSegments: tt.hourSegments,
			}
			pc := NewPathCalculator(cfg)
			got := pc.CalculateHourSegment(tt.minute)
			if got != tt.want {
				t.Errorf("CalculateHourSegment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathCalculator_CalculateWorkSegment(t *testing.T) {
	tests := []struct {
		name            string
		dayWorkSegments int
		hour            int
		want            int
	}{
		{
			name:            "不使用工作段",
			dayWorkSegments: 0,
			hour:            12,
			want:            0,
		},
		{
			name:            "3段配置 - 第1段（0-7小时）",
			dayWorkSegments: 3,
			hour:            0,
			want:            1,
		},
		{
			name:            "3段配置 - 第2段（8-15小时）",
			dayWorkSegments: 3,
			hour:            8,
			want:            2,
		},
		{
			name:            "3段配置 - 第3段（16-23小时）",
			dayWorkSegments: 3,
			hour:            16,
			want:            3,
		},
		{
			name:            "4段配置 - 第1段",
			dayWorkSegments: 4,
			hour:            0,
			want:            1,
		},
		{
			name:            "4段配置 - 第2段",
			dayWorkSegments: 4,
			hour:            6,
			want:            2,
		},
		{
			name:            "4段配置 - 第3段",
			dayWorkSegments: 4,
			hour:            12,
			want:            3,
		},
		{
			name:            "4段配置 - 第4段",
			dayWorkSegments: 4,
			hour:            18,
			want:            4,
		},
		{
			name:            "边界测试 - 负数小时",
			dayWorkSegments: 3,
			hour:            -1,
			want:            1,
		},
		{
			name:            "边界测试 - 超过24小时",
			dayWorkSegments: 3,
			hour:            25,
			want:            3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StorageConfig{
				DayWorkSegments: tt.dayWorkSegments,
			}
			pc := NewPathCalculator(cfg)
			got := pc.CalculateWorkSegment(tt.hour)
			if got != tt.want {
				t.Errorf("CalculateWorkSegment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathCalculator_CalculateWeek(t *testing.T) {
	tests := []struct {
		name       string
		monthWeeks string
		year       int
		month      int
		day        int
		want       int
	}{
		{
			name:       "calendar模式 - 第1周",
			monthWeeks: "calendar",
			year:       2025,
			month:      1,
			day:        1,
			want:       1,
		},
		{
			name:       "calendar模式 - 第2周",
			monthWeeks: "calendar",
			year:       2025,
			month:      1,
			day:        8,
			want:       2,
		},
		{
			name:       "calendar模式 - 第3周",
			monthWeeks: "calendar",
			year:       2025,
			month:      1,
			day:        15,
			want:       3,
		},
		{
			name:       "calendar模式 - 第4周",
			monthWeeks: "calendar",
			year:       2025,
			month:      1,
			day:        22,
			want:       4,
		},
		{
			name:       "calendar模式 - 第5周",
			monthWeeks: "calendar",
			year:       2025,
			month:      1,
			day:        29,
			want:       5,
		},
		{
			name:       "fixed模式 - 月初",
			monthWeeks: "fixed",
			year:       2025,
			month:      1,
			day:        1,
			want:       1,
		},
		{
			name:       "fixed模式 - 月中",
			monthWeeks: "fixed",
			year:       2025,
			month:      1,
			day:        15,
			want:       3,
		},
		{
			name:       "fixed模式 - 月末",
			monthWeeks: "fixed",
			year:       2025,
			month:      1,
			day:        31,
			want:       5,
		},
		{
			name:       "边界测试 - 日期为0",
			monthWeeks: "calendar",
			year:       2025,
			month:      1,
			day:        0,
			want:       1,
		},
		{
			name:       "边界测试 - 日期超过月份天数",
			monthWeeks: "calendar",
			year:       2025,
			month:      2,
			day:        35,
			want:       4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StorageConfig{
				MonthWeeks: tt.monthWeeks,
			}
			pc := NewPathCalculator(cfg)
			got := pc.CalculateWeek(tt.year, tt.month, tt.day)
			if got != tt.want {
				t.Errorf("CalculateWeek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathCalculator_CalculateQuarter(t *testing.T) {
	tests := []struct {
		name         string
		yearQuarters int
		month        int
		want         int
	}{
		{
			name:         "4季度配置 - Q1（1-3月）",
			yearQuarters: 4,
			month:        1,
			want:         1,
		},
		{
			name:         "4季度配置 - Q1边界",
			yearQuarters: 4,
			month:        3,
			want:         1,
		},
		{
			name:         "4季度配置 - Q2（4-6月）",
			yearQuarters: 4,
			month:        4,
			want:         2,
		},
		{
			name:         "4季度配置 - Q3（7-9月）",
			yearQuarters: 4,
			month:        7,
			want:         3,
		},
		{
			name:         "4季度配置 - Q4（10-12月）",
			yearQuarters: 4,
			month:        10,
			want:         4,
		},
		{
			name:         "2季度配置 - 上半年",
			yearQuarters: 2,
			month:        1,
			want:         1,
		},
		{
			name:         "2季度配置 - 下半年",
			yearQuarters: 2,
			month:        7,
			want:         2,
		},
		{
			name:         "边界测试 - 月份为0",
			yearQuarters: 4,
			month:        0,
			want:         1,
		},
		{
			name:         "边界测试 - 月份超过12",
			yearQuarters: 4,
			month:        13,
			want:         4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StorageConfig{
				YearQuarters: tt.yearQuarters,
			}
			pc := NewPathCalculator(cfg)
			got := pc.CalculateQuarter(tt.month)
			if got != tt.want {
				t.Errorf("CalculateQuarter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathCalculator_FormatDirs(t *testing.T) {
	cfg := &config.StorageConfig{}
	pc := NewPathCalculator(cfg)

	tests := []struct {
		name     string
		function func(int) string
		input    int
		want     string
	}{
		{
			name:     "FormatSegmentDir - S1",
			function: pc.FormatSegmentDir,
			input:    1,
			want:     "S1",
		},
		{
			name:     "FormatSegmentDir - S4",
			function: pc.FormatSegmentDir,
			input:    4,
			want:     "S4",
		},
		{
			name:     "FormatWorkSegmentDir - WS1",
			function: pc.FormatWorkSegmentDir,
			input:    1,
			want:     "WS1",
		},
		{
			name:     "FormatWorkSegmentDir - WS3",
			function: pc.FormatWorkSegmentDir,
			input:    3,
			want:     "WS3",
		},
		{
			name:     "FormatWeekDir - W1",
			function: pc.FormatWeekDir,
			input:    1,
			want:     "W1",
		},
		{
			name:     "FormatWeekDir - W5",
			function: pc.FormatWeekDir,
			input:    5,
			want:     "W5",
		},
		{
			name:     "FormatQuarterDir - Q1",
			function: pc.FormatQuarterDir,
			input:    1,
			want:     "Q1",
		},
		{
			name:     "FormatQuarterDir - Q4",
			function: pc.FormatQuarterDir,
			input:    4,
			want:     "Q4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.function(tt.input)
			if got != tt.want {
				t.Errorf("Format function = %v, want %v", got, tt.want)
			}
		})
	}
}

// 测试路径计算的确定性：相同输入应该产生相同输出
func TestPathCalculator_Deterministic(t *testing.T) {
	cfg := &config.StorageConfig{
		HourSegments:    4,
		DayWorkSegments: 3,
		MonthWeeks:      "calendar",
		YearQuarters:    4,
	}
	pc := NewPathCalculator(cfg)

	// 多次计算相同的输入，应该得到相同的结果
	for i := 0; i < 10; i++ {
		seg1 := pc.CalculateHourSegment(15)
		seg2 := pc.CalculateHourSegment(15)
		if seg1 != seg2 {
			t.Errorf("CalculateHourSegment not deterministic: %d != %d", seg1, seg2)
		}

		ws1 := pc.CalculateWorkSegment(12)
		ws2 := pc.CalculateWorkSegment(12)
		if ws1 != ws2 {
			t.Errorf("CalculateWorkSegment not deterministic: %d != %d", ws1, ws2)
		}

		w1 := pc.CalculateWeek(2025, 1, 15)
		w2 := pc.CalculateWeek(2025, 1, 15)
		if w1 != w2 {
			t.Errorf("CalculateWeek not deterministic: %d != %d", w1, w2)
		}

		q1 := pc.CalculateQuarter(6)
		q2 := pc.CalculateQuarter(6)
		if q1 != q2 {
			t.Errorf("CalculateQuarter not deterministic: %d != %d", q1, q2)
		}
	}
}

// 测试分段号的有效范围
func TestPathCalculator_SegmentRange(t *testing.T) {
	tests := []struct {
		name         string
		hourSegments int
	}{
		{"1段", 1},
		{"2段", 2},
		{"3段", 3},
		{"4段", 4},
		{"6段", 6},
		{"12段", 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StorageConfig{
				HourSegments: tt.hourSegments,
			}
			pc := NewPathCalculator(cfg)

			// 测试所有分钟数
			for minute := 0; minute < 60; minute++ {
				seg := pc.CalculateHourSegment(minute)
				if seg < 1 || seg > tt.hourSegments {
					t.Errorf("CalculateHourSegment(%d) = %d, out of range [1, %d]", minute, seg, tt.hourSegments)
				}
			}
		})
	}
}

func TestPathCalculator_BuildPath(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.StorageConfig
		time     time.Time
		fileType FileType
		want     string
	}{
		{
			name: "截图文件 - 完整层级",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeScreenshot,
			want:     "2025/Q1/01/W3/15/WS2/10/S3/30.png",
		},
		{
			name: "报告文件 - 完整层级",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeReport,
			want:     "2025/Q1/01/W3/15/WS2/10/S3/30.md",
		},
		{
			name: "小时内分段汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummarySegment,
			want:     "2025/Q1/01/W3/15/WS2/10/S3/summary.md",
		},
		{
			name: "小时汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummaryHour,
			want:     "2025/Q1/01/W3/15/WS2/10/hour.md",
		},
		{
			name: "工作段汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummaryWorkSegment,
			want:     "2025/Q1/01/W3/15/WS2/work-segment.md",
		},
		{
			name: "天汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummaryDay,
			want:     "2025/Q1/01/W3/15/day.md",
		},
		{
			name: "周汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummaryWeek,
			want:     "2025/Q1/01/W3/week.md",
		},
		{
			name: "月汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummaryMonth,
			want:     "2025/Q1/01/month.md",
		},
		{
			name: "季度汇总",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeSummaryQuarter,
			want:     "2025/Q1/quarter.md",
		},
		{
			name: "跳过季度层级（只有1个季度）",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    1,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeScreenshot,
			want:     "2025/01/W3/15/WS2/10/S3/30.png",
		},
		{
			name: "跳过工作段层级（不使用工作段）",
			config: &config.StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeScreenshot,
			want:     "2025/Q1/01/W3/15/10/S3/30.png",
		},
		{
			name: "跳过小时分段层级（只有1个分段）",
			config: &config.StorageConfig{
				HourSegments:    1,
				DayWorkSegments: 3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeScreenshot,
			want:     "2025/Q1/01/W3/15/WS2/10/30.png",
		},
		{
			name: "最简配置 - 只有客观周期",
			config: &config.StorageConfig{
				HourSegments:    1,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    1,
			},
			time:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			fileType: FileTypeScreenshot,
			want:     "2025/01/W3/15/10/30.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPathCalculator(tt.config)
			got := pc.BuildPath(tt.time, tt.fileType)
			if got != tt.want {
				t.Errorf("BuildPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 测试路径格式的一致性
func TestPathCalculator_PathFormatConsistency(t *testing.T) {
	cfg := &config.StorageConfig{
		HourSegments:    4,
		DayWorkSegments: 3,
		MonthWeeks:      "calendar",
		YearQuarters:    4,
	}
	pc := NewPathCalculator(cfg)

	testTime := time.Date(2025, 6, 15, 14, 45, 0, 0, time.UTC)

	// 测试所有文件类型都能生成有效路径
	fileTypes := []FileType{
		FileTypeScreenshot,
		FileTypeReport,
		FileTypeSummarySegment,
		FileTypeSummaryHour,
		FileTypeSummaryWorkSegment,
		FileTypeSummaryDay,
		FileTypeSummaryWeek,
		FileTypeSummaryMonth,
		FileTypeSummaryQuarter,
	}

	for _, ft := range fileTypes {
		path := pc.BuildPath(testTime, ft)
		if path == "" {
			t.Errorf("BuildPath returned empty path for file type %d", ft)
		}
		// 验证路径包含年份
		if len(path) < 4 || path[:4] != "2025" {
			t.Errorf("BuildPath path does not start with year: %s", path)
		}
	}
}

// 测试目录层级的完整性
func TestPathCalculator_DirectoryHierarchy(t *testing.T) {
	cfg := &config.StorageConfig{
		HourSegments:    4,
		DayWorkSegments: 3,
		MonthWeeks:      "calendar",
		YearQuarters:    4,
	}
	pc := NewPathCalculator(cfg)

	testTime := time.Date(2025, 3, 20, 16, 25, 0, 0, time.UTC)
	path := pc.BuildPath(testTime, FileTypeScreenshot)

	// 验证路径包含所有必需的层级
	expectedComponents := []string{
		"2025",   // 年份
		"Q1",     // 季度
		"03",     // 月份
		"W3",     // 周
		"20",     // 日期
		"WS3",    // 工作段
		"16",     // 小时
		"S2",     // 分段
		"25.png", // 文件名
	}

	for _, component := range expectedComponents {
		if !contains(path, component) {
			t.Errorf("Path %s does not contain expected component %s", path, component)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// 属性测试 (Property-Based Tests)
// ============================================================================

// 属性 1：路径计算的确定性
// 对于任意给定的时间戳和配置，路径计算器应该始终返回相同的路径
// 验证：需求 2.4, 11.1-11.4

// TestProperty1_PathCalculationDeterminism 测试路径计算的确定性
func TestProperty1_PathCalculationDeterminism(t *testing.T) {
	// 定义测试配置集合
	configs := []config.StorageConfig{
		{HourSegments: 1, DayWorkSegments: 0, MonthWeeks: "calendar", YearQuarters: 4},
		{HourSegments: 2, DayWorkSegments: 0, MonthWeeks: "calendar", YearQuarters: 4},
		{HourSegments: 4, DayWorkSegments: 0, MonthWeeks: "calendar", YearQuarters: 4},
		{HourSegments: 4, DayWorkSegments: 3, MonthWeeks: "fixed", YearQuarters: 2},
		{HourSegments: 6, DayWorkSegments: 6, MonthWeeks: "calendar", YearQuarters: 12},
	}

	// 定义测试时间戳集合（覆盖各种边界情况）
	timestamps := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),     // 年初
		time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC), // 年末
		time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC),  // 年中
		time.Date(2025, 2, 28, 23, 59, 0, 0, time.UTC),  // 2月末
		time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),   // 闰年2月29日
		time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),    // 31天月份末
		time.Date(2025, 4, 30, 23, 59, 0, 0, time.UTC),  // 30天月份末
	}

	// 定义文件类型集合
	fileTypes := []FileType{
		FileTypeScreenshot,
		FileTypeReport,
		FileTypeSummarySegment,
		FileTypeSummaryHour,
		FileTypeSummaryWorkSegment,
		FileTypeSummaryDay,
		FileTypeSummaryWeek,
		FileTypeSummaryMonth,
		FileTypeSummaryQuarter,
	}

	testCount := 0
	for _, cfg := range configs {
		calc := NewPathCalculator(&cfg)

		for _, ts := range timestamps {
			for _, ft := range fileTypes {
				testCount++

				// 计算路径多次，验证结果一致
				path1 := calc.BuildPath(ts, ft)
				path2 := calc.BuildPath(ts, ft)
				path3 := calc.BuildPath(ts, ft)

				if path1 != path2 || path2 != path3 {
					t.Errorf("路径计算不确定: 时间=%v, 类型=%v, 配置=%+v\n  第1次=%s\n  第2次=%s\n  第3次=%s",
						ts, ft, cfg, path1, path2, path3)
				}

				// 验证相同输入产生相同输出（使用新的计算器实例）
				calc2 := NewPathCalculator(&cfg)
				path4 := calc2.BuildPath(ts, ft)

				if path1 != path4 {
					t.Errorf("不同计算器实例产生不同路径: 时间=%v, 类型=%v\n  实例1=%s\n  实例2=%s",
						ts, ft, path1, path4)
				}
			}
		}
	}

	t.Logf("属性 1 测试完成: 总计 %d 个测试用例", testCount)
}

// TestProperty1_SegmentCalculationDeterminism 测试分段号计算的确定性
func TestProperty1_SegmentCalculationDeterminism(t *testing.T) {
	configs := []config.StorageConfig{
		{HourSegments: 1, DayWorkSegments: 0, MonthWeeks: "calendar", YearQuarters: 4},
		{HourSegments: 4, DayWorkSegments: 0, MonthWeeks: "calendar", YearQuarters: 4},
		{HourSegments: 12, DayWorkSegments: 8, MonthWeeks: "fixed", YearQuarters: 2},
	}

	testCount := 0
	for _, cfg := range configs {
		calc := NewPathCalculator(&cfg)

		// 测试所有可能的分钟值
		for minute := 0; minute < 60; minute++ {
			testCount++

			seg1 := calc.CalculateHourSegment(minute)
			seg2 := calc.CalculateHourSegment(minute)
			seg3 := calc.CalculateHourSegment(minute)

			if seg1 != seg2 || seg2 != seg3 {
				t.Errorf("小时分段计算不确定: 分钟=%d, 配置=%+v, 结果=%d,%d,%d",
					minute, cfg, seg1, seg2, seg3)
			}
		}

		// 测试所有可能的小时值
		for hour := 0; hour < 24; hour++ {
			testCount++

			ws1 := calc.CalculateWorkSegment(hour)
			ws2 := calc.CalculateWorkSegment(hour)
			ws3 := calc.CalculateWorkSegment(hour)

			if ws1 != ws2 || ws2 != ws3 {
				t.Errorf("工作段计算不确定: 小时=%d, 配置=%+v, 结果=%d,%d,%d",
					hour, cfg, ws1, ws2, ws3)
			}
		}

		// 测试各种日期的周数计算
		for month := 1; month <= 12; month++ {
			daysInMonth := time.Date(2025, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
			for day := 1; day <= daysInMonth; day++ {
				testCount++

				w1 := calc.CalculateWeek(2025, month, day)
				w2 := calc.CalculateWeek(2025, month, day)
				w3 := calc.CalculateWeek(2025, month, day)

				if w1 != w2 || w2 != w3 {
					t.Errorf("周数计算不确定: 年=%d, 月=%d, 日=%d, 配置=%+v, 结果=%d,%d,%d",
						2025, month, day, cfg, w1, w2, w3)
				}
			}
		}

		// 测试所有月份的季度计算
		for month := 1; month <= 12; month++ {
			testCount++

			q1 := calc.CalculateQuarter(month)
			q2 := calc.CalculateQuarter(month)
			q3 := calc.CalculateQuarter(month)

			if q1 != q2 || q2 != q3 {
				t.Errorf("季度计算不确定: 月份=%d, 配置=%+v, 结果=%d,%d,%d",
					month, cfg, q1, q2, q3)
			}
		}
	}

	t.Logf("属性 1（分段计算）测试完成: 总计 %d 个测试用例", testCount)
}

// 属性 2：分段号的有效范围
// 对于任意有效的输入值，计算出的分段号应该在有效范围内（1 到 分段数量）
// 验证：需求 2.4, 11.7

// TestProperty2_SegmentNumberRange 测试分段号的有效范围
func TestProperty2_SegmentNumberRange(t *testing.T) {
	// 所有有效的分段数配置
	validHourSegments := []int{1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60}
	validWorkSegments := []int{0, 1, 2, 3, 4, 6, 8, 12, 24}
	validQuarters := []int{1, 2, 3, 4, 6, 12}

	testCount := 0

	// 测试小时分段号范围
	for _, segments := range validHourSegments {
		cfg := config.StorageConfig{
			HourSegments:    segments,
			DayWorkSegments: 0,
			MonthWeeks:      "calendar",
			YearQuarters:    4,
		}
		calc := NewPathCalculator(&cfg)

		for minute := 0; minute < 60; minute++ {
			testCount++
			seg := calc.CalculateHourSegment(minute)

			if seg < 1 || seg > segments {
				t.Errorf("小时分段号超出范围: 分钟=%d, 分段数=%d, 计算结果=%d, 期望范围=[1,%d]",
					minute, segments, seg, segments)
			}
		}
	}

	// 测试工作段号范围
	for _, segments := range validWorkSegments {
		if segments == 0 {
			continue // 0 表示不使用工作段
		}

		cfg := config.StorageConfig{
			HourSegments:    4,
			DayWorkSegments: segments,
			MonthWeeks:      "calendar",
			YearQuarters:    4,
		}
		calc := NewPathCalculator(&cfg)

		for hour := 0; hour < 24; hour++ {
			testCount++
			ws := calc.CalculateWorkSegment(hour)

			if ws < 1 || ws > segments {
				t.Errorf("工作段号超出范围: 小时=%d, 分段数=%d, 计算结果=%d, 期望范围=[1,%d]",
					hour, segments, ws, segments)
			}
		}
	}

	// 测试周号范围（calendar 模式）
	cfg := config.StorageConfig{
		HourSegments:    4,
		DayWorkSegments: 0,
		MonthWeeks:      "calendar",
		YearQuarters:    4,
	}
	calc := NewPathCalculator(&cfg)

	for month := 1; month <= 12; month++ {
		daysInMonth := time.Date(2025, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		maxWeek := (daysInMonth-1)/7 + 1

		for day := 1; day <= daysInMonth; day++ {
			testCount++
			week := calc.CalculateWeek(2025, month, day)

			if week < 1 || week > maxWeek {
				t.Errorf("周号超出范围(calendar): 月=%d, 日=%d, 当月天数=%d, 计算结果=%d, 期望范围=[1,%d]",
					month, day, daysInMonth, week, maxWeek)
			}
		}
	}

	// 测试季度号范围
	for _, quarters := range validQuarters {
		cfg := config.StorageConfig{
			HourSegments:    4,
			DayWorkSegments: 0,
			MonthWeeks:      "calendar",
			YearQuarters:    quarters,
		}
		calc := NewPathCalculator(&cfg)

		for month := 1; month <= 12; month++ {
			testCount++
			q := calc.CalculateQuarter(month)

			if q < 1 || q > quarters {
				t.Errorf("季度号超出范围: 月=%d, 季度数=%d, 计算结果=%d, 期望范围=[1,%d]",
					month, quarters, q, quarters)
			}
		}
	}

	t.Logf("属性 2 测试完成: 总计 %d 个测试用例", testCount)
}

// TestProperty2_SegmentNumberCoverage 测试分段号覆盖所有可能值
func TestProperty2_SegmentNumberCoverage(t *testing.T) {
	// 验证每个分段号都会被使用到
	testCases := []struct {
		name     string
		segments int
	}{
		{"2段", 2},
		{"3段", 3},
		{"4段", 4},
		{"6段", 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.StorageConfig{
				HourSegments:    tc.segments,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			}
			calc := NewPathCalculator(&cfg)

			// 记录每个分段号是否被使用
			used := make(map[int]bool)

			for minute := 0; minute < 60; minute++ {
				seg := calc.CalculateHourSegment(minute)
				used[seg] = true
			}

			// 验证所有分段号都被使用
			for i := 1; i <= tc.segments; i++ {
				if !used[i] {
					t.Errorf("分段号 %d 未被使用（总共 %d 段）", i, tc.segments)
				}
			}

			// 验证没有使用超出范围的分段号
			for seg := range used {
				if seg < 1 || seg > tc.segments {
					t.Errorf("使用了超出范围的分段号: %d（期望范围 [1,%d]）", seg, tc.segments)
				}
			}
		})
	}
}

// 属性 3：文件名格式的一致性
// 对于任意新生成的文件，文件名应该使用统一的 MIN 格式（两位数分钟）
// 验证：需求 1.1, 1.2

// TestProperty3_FileNameFormatConsistency 测试文件名格式的一致性
func TestProperty3_FileNameFormatConsistency(t *testing.T) {
	cfg := config.StorageConfig{
		HourSegments:    4,
		DayWorkSegments: 0,
		MonthWeeks:      "calendar",
		YearQuarters:    4,
	}
	calc := NewPathCalculator(&cfg)

	testCount := 0

	// 测试所有分钟值的文件名格式
	for minute := 0; minute < 60; minute++ {
		ts := time.Date(2025, 6, 15, 12, minute, 0, 0, time.UTC)

		// 测试截图文件名
		testCount++
		screenshotPath := calc.BuildPath(ts, FileTypeScreenshot)
		if !isValidMinuteFormat(screenshotPath, ".png") {
			t.Errorf("截图文件名格式不正确: 分钟=%d, 路径=%s", minute, screenshotPath)
		}

		// 测试报告文件名
		testCount++
		reportPath := calc.BuildPath(ts, FileTypeReport)
		if !isValidMinuteFormat(reportPath, ".md") {
			t.Errorf("报告文件名格式不正确: 分钟=%d, 路径=%s", minute, reportPath)
		}
	}

	t.Logf("属性 3 测试完成: 总计 %d 个测试用例", testCount)
}

// isValidMinuteFormat 检查路径是否以正确的分钟格式结尾（两位数 + 扩展名）
func isValidMinuteFormat(path, ext string) bool {
	// 提取文件名
	parts := splitPath(path)
	if len(parts) == 0 {
		return false
	}

	fileName := parts[len(parts)-1]

	// 检查是否以扩展名结尾
	if len(fileName) < len(ext) || fileName[len(fileName)-len(ext):] != ext {
		return false
	}

	// 提取分钟部分（去掉扩展名）
	minutePart := fileName[:len(fileName)-len(ext)]

	// 验证是两位数字
	if len(minutePart) != 2 {
		return false
	}

	for _, c := range minutePart {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// splitPath 分割路径
func splitPath(path string) []string {
	var parts []string
	current := ""

	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// 属性 4：目录层级的完整性
// 对于任意文件路径，所有必需的目录层级应该按照从大到小的顺序存在
// 验证：需求 4.1

// TestProperty4_DirectoryHierarchyCompleteness 测试目录层级的完整性
func TestProperty4_DirectoryHierarchyCompleteness(t *testing.T) {
	cfg := config.StorageConfig{
		HourSegments:    4,
		DayWorkSegments: 3,
		MonthWeeks:      "calendar",
		YearQuarters:    4,
	}
	calc := NewPathCalculator(&cfg)

	testCases := []struct {
		name     string
		fileType FileType
		expected []string // 期望的层级顺序
	}{
		{
			name:     "截图文件",
			fileType: FileTypeScreenshot,
			expected: []string{"YYYY", "QN", "MM", "WN", "DD", "WSN", "HH", "SN", "MIN.png"},
		},
		{
			name:     "报告文件",
			fileType: FileTypeReport,
			expected: []string{"YYYY", "QN", "MM", "WN", "DD", "WSN", "HH", "SN", "MIN.md"},
		},
		{
			name:     "分段汇总",
			fileType: FileTypeSummarySegment,
			expected: []string{"YYYY", "QN", "MM", "WN", "DD", "WSN", "HH", "SN", "summary.md"},
		},
		{
			name:     "小时汇总",
			fileType: FileTypeSummaryHour,
			expected: []string{"YYYY", "QN", "MM", "WN", "DD", "WSN", "HH", "hour.md"},
		},
		{
			name:     "工作段汇总",
			fileType: FileTypeSummaryWorkSegment,
			expected: []string{"YYYY", "QN", "MM", "WN", "DD", "WSN", "work-segment.md"},
		},
		{
			name:     "天汇总",
			fileType: FileTypeSummaryDay,
			expected: []string{"YYYY", "QN", "MM", "WN", "DD", "day.md"},
		},
		{
			name:     "周汇总",
			fileType: FileTypeSummaryWeek,
			expected: []string{"YYYY", "QN", "MM", "WN", "week.md"},
		},
		{
			name:     "月汇总",
			fileType: FileTypeSummaryMonth,
			expected: []string{"YYYY", "QN", "MM", "month.md"},
		},
		{
			name:     "季度汇总",
			fileType: FileTypeSummaryQuarter,
			expected: []string{"YYYY", "QN", "quarter.md"},
		},
	}

	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := calc.BuildPath(ts, tc.fileType)
			parts := splitPath(path)

			// 验证层级数量
			if len(parts) != len(tc.expected) {
				t.Errorf("层级数量不匹配: 期望=%d, 实际=%d, 路径=%s",
					len(tc.expected), len(parts), path)
				return
			}

			// 验证每个层级的格式
			for i, expectedPattern := range tc.expected {
				actualPart := parts[i]

				if !matchesPattern(actualPart, expectedPattern) {
					t.Errorf("层级 %d 格式不匹配: 期望模式=%s, 实际=%s, 完整路径=%s",
						i, expectedPattern, actualPart, path)
				}
			}
		})
	}
}

// matchesPattern 检查实际值是否匹配期望的模式
func matchesPattern(actual, pattern string) bool {
	switch pattern {
	case "YYYY":
		return len(actual) == 4 && isAllDigits(actual)
	case "QN":
		return len(actual) >= 2 && actual[0] == 'Q' && isAllDigits(actual[1:])
	case "MM":
		return len(actual) == 2 && isAllDigits(actual)
	case "WN":
		return len(actual) >= 2 && actual[0] == 'W' && isAllDigits(actual[1:])
	case "DD":
		return len(actual) == 2 && isAllDigits(actual)
	case "WSN":
		return len(actual) >= 3 && actual[0:2] == "WS" && isAllDigits(actual[2:])
	case "HH":
		return len(actual) == 2 && isAllDigits(actual)
	case "SN":
		return len(actual) >= 2 && actual[0] == 'S' && isAllDigits(actual[1:])
	case "MIN.png", "MIN.md":
		// 检查文件名格式：两位数字 + 扩展名
		ext := pattern[3:] // .png 或 .md
		if len(actual) < len(ext)+2 {
			return false
		}
		if actual[len(actual)-len(ext):] != ext {
			return false
		}
		minutePart := actual[:len(actual)-len(ext)]
		return len(minutePart) == 2 && isAllDigits(minutePart)
	default:
		// 对于其他文件名，直接比较
		return actual == pattern
	}
}

// isAllDigits 检查字符串是否全是数字
func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
