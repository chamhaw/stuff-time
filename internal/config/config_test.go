package config

import (
	"testing"
)

func TestStorageConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  StorageConfig
		wantErr bool
	}{
		{
			name: "有效配置 - 默认值",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			wantErr: false,
		},
		{
			name: "有效配置 - 小时分段为2",
			config: StorageConfig{
				HourSegments:    2,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			wantErr: false,
		},
		{
			name: "有效配置 - 使用工作段",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 3,
				MonthWeeks:      "fixed",
				YearQuarters:    4,
			},
			wantErr: false,
		},
		{
			name: "无效配置 - 小时分段不能整除60",
			config: StorageConfig{
				HourSegments:    7,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			wantErr: true,
		},
		{
			name: "无效配置 - 工作段不能整除24",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 5,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			wantErr: true,
		},
		{
			name: "无效配置 - 季度数不能整除12",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    5,
			},
			wantErr: true,
		},
		{
			name: "无效配置 - 月周计算方式无效",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "invalid",
				YearQuarters:    4,
			},
			wantErr: true,
		},
		{
			name: "无效配置 - 小时分段为负数",
			config: StorageConfig{
				HourSegments:    -1,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			wantErr: true,
		},
		{
			name: "无效配置 - 工作段为负数",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: -1,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("StorageConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStorageConfig_ApplyDefaults(t *testing.T) {
	config := StorageConfig{}
	config.ApplyDefaults()

	if config.HourSegments != 4 {
		t.Errorf("Expected HourSegments to be 4, got %d", config.HourSegments)
	}
	if config.DayWorkSegments != 0 {
		t.Errorf("Expected DayWorkSegments to be 0, got %d", config.DayWorkSegments)
	}
	if config.MonthWeeks != "calendar" {
		t.Errorf("Expected MonthWeeks to be 'calendar', got '%s'", config.MonthWeeks)
	}
	if config.YearQuarters != 4 {
		t.Errorf("Expected YearQuarters to be 4, got %d", config.YearQuarters)
	}
}

func TestStorageConfig_ValidSegmentCounts(t *testing.T) {
	// 测试所有有效的小时分段数
	validHourSegments := []int{1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60}
	for _, segments := range validHourSegments {
		config := StorageConfig{
			HourSegments:    segments,
			DayWorkSegments: 0,
			MonthWeeks:      "calendar",
			YearQuarters:    4,
		}
		if err := config.Validate(); err != nil {
			t.Errorf("Expected hour_segments=%d to be valid, got error: %v", segments, err)
		}
	}

	// 测试所有有效的工作段数
	validWorkSegments := []int{0, 1, 2, 3, 4, 6, 8, 12, 24}
	for _, segments := range validWorkSegments {
		config := StorageConfig{
			HourSegments:    4,
			DayWorkSegments: segments,
			MonthWeeks:      "calendar",
			YearQuarters:    4,
		}
		if err := config.Validate(); err != nil {
			t.Errorf("Expected day_work_segments=%d to be valid, got error: %v", segments, err)
		}
	}

	// 测试所有有效的季度数
	validQuarters := []int{1, 2, 3, 4, 6, 12}
	for _, quarters := range validQuarters {
		config := StorageConfig{
			HourSegments:    4,
			DayWorkSegments: 0,
			MonthWeeks:      "calendar",
			YearQuarters:    quarters,
		}
		if err := config.Validate(); err != nil {
			t.Errorf("Expected year_quarters=%d to be valid, got error: %v", quarters, err)
		}
	}
}

// ============================================================================
// 属性测试 (Property-Based Tests)
// ============================================================================

// 属性 8：配置验证的完整性
// 对于任意无效的配置值，系统应该拒绝启动或使用默认值，并记录警告
// 验证：需求 2.3, 5.5, 12.5

// TestProperty8_ConfigValidationCompleteness 测试配置验证的完整性
// 使用穷举测试代替随机测试，确保覆盖所有重要的边界情况
func TestProperty8_ConfigValidationCompleteness(t *testing.T) {
	// 定义测试值集合
	hourSegmentValues := []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 15, 20, 30, 60, 100}
	workSegmentValues := []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 24, 100}
	quarterValues := []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 12, 24}
	weekModeValues := []string{"", "calendar", "fixed", "invalid", "weekly"}

	testCount := 0
	validCount := 0
	invalidCount := 0

	// 穷举测试所有组合
	for _, hourSeg := range hourSegmentValues {
		for _, workSeg := range workSegmentValues {
			for _, quarters := range quarterValues {
				for _, weekMode := range weekModeValues {
					testCount++

					config := StorageConfig{
						HourSegments:    hourSeg,
						DayWorkSegments: workSeg,
						MonthWeeks:      weekMode,
						YearQuarters:    quarters,
					}

					err := config.Validate()

					// 检查小时分段是否有效
					hourSegValid := hourSeg > 0 && 60%hourSeg == 0

					// 检查工作段是否有效
					workSegValid := workSeg >= 0 && (workSeg == 0 || 24%workSeg == 0)

					// 检查季度数是否有效
					quartersValid := quarters > 0 && 12%quarters == 0

					// 检查月周模式是否有效
					weekModeValid := weekMode == "calendar" || weekMode == "fixed"

					// 所有配置项都有效时，验证应该通过
					allValid := hourSegValid && workSegValid && quartersValid && weekModeValid

					if allValid {
						validCount++
						if err != nil {
							t.Errorf("有效配置被错误拒绝: hourSeg=%d, workSeg=%d, quarters=%d, weekMode=%s, err=%v",
								hourSeg, workSeg, quarters, weekMode, err)
						}
					} else {
						invalidCount++
						if err == nil {
							t.Errorf("无效配置未被拒绝: hourSeg=%d, workSeg=%d, quarters=%d, weekMode=%s",
								hourSeg, workSeg, quarters, weekMode)
						}
					}
				}
			}
		}
	}

	t.Logf("属性 8 测试完成: 总计 %d 个测试用例, %d 个有效配置, %d 个无效配置",
		testCount, validCount, invalidCount)
}

// TestProperty8_DefaultsAreValid 测试默认配置总是有效的
func TestProperty8_DefaultsAreValid(t *testing.T) {
	// 属性：应用默认值后的配置总是有效的
	property := func() bool {
		config := StorageConfig{}
		config.ApplyDefaults()
		return config.Validate() == nil
	}

	// 运行 100 次以确保稳定性
	for i := 0; i < 100; i++ {
		if !property() {
			t.Errorf("默认配置在第 %d 次迭代时无效", i+1)
			return
		}
	}
}

// TestProperty8_InvalidConfigsRejected 测试所有已知的无效配置都被拒绝
func TestProperty8_InvalidConfigsRejected(t *testing.T) {
	invalidConfigs := []struct {
		name   string
		config StorageConfig
	}{
		{
			name: "小时分段为 0",
			config: StorageConfig{
				HourSegments:    0,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
		},
		{
			name: "小时分段为负数",
			config: StorageConfig{
				HourSegments:    -5,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
		},
		{
			name: "小时分段不能整除 60",
			config: StorageConfig{
				HourSegments:    7,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
		},
		{
			name: "工作段为负数",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: -3,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
		},
		{
			name: "工作段不能整除 24",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 5,
				MonthWeeks:      "calendar",
				YearQuarters:    4,
			},
		},
		{
			name: "季度数为 0",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    0,
			},
		},
		{
			name: "季度数为负数",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    -2,
			},
		},
		{
			name: "季度数不能整除 12",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "calendar",
				YearQuarters:    5,
			},
		},
		{
			name: "月周模式为空",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "",
				YearQuarters:    4,
			},
		},
		{
			name: "月周模式无效",
			config: StorageConfig{
				HourSegments:    4,
				DayWorkSegments: 0,
				MonthWeeks:      "invalid",
				YearQuarters:    4,
			},
		},
	}

	for _, tc := range invalidConfigs {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if err == nil {
				t.Errorf("预期配置 %s 应该被拒绝，但验证通过了", tc.name)
			}
		})
	}
}
