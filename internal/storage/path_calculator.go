package storage

import (
	"fmt"
	"path/filepath"
	"time"

	"stuff-time/internal/config"
)

// FileType 文件类型
type FileType int

const (
	FileTypeScreenshot FileType = iota
	FileTypeReport
	FileTypeSummarySegment
	FileTypeSummaryHour
	FileTypeSummaryWorkSegment
	FileTypeSummaryDay
	FileTypeSummaryWeek
	FileTypeSummaryMonth
	FileTypeSummaryQuarter
)

// PathCalculator 路径计算器，根据时间戳和配置计算文件路径
type PathCalculator struct {
	config *config.StorageConfig
}

// NewPathCalculator 创建路径计算器
func NewPathCalculator(cfg *config.StorageConfig) *PathCalculator {
	return &PathCalculator{
		config: cfg,
	}
}

// CalculateHourSegment 计算小时内分段号（1-based）
// 公式：段号 = floor(分钟数 * hour_segments / 60) + 1
func (pc *PathCalculator) CalculateHourSegment(minute int) int {
	if pc.config.HourSegments <= 0 {
		return 1
	}
	if pc.config.HourSegments == 1 {
		return 1
	}

	// 确保分钟数在有效范围内
	if minute < 0 {
		minute = 0
	}
	if minute >= 60 {
		minute = 59
	}

	segmentNum := (minute * pc.config.HourSegments / 60) + 1

	// 确保结果在有效范围内
	if segmentNum < 1 {
		segmentNum = 1
	}
	if segmentNum > pc.config.HourSegments {
		segmentNum = pc.config.HourSegments
	}

	return segmentNum
}

// CalculateWorkSegment 计算日内工作段号（1-based）
// 公式：段号 = floor(小时数 * day_work_segments / 24) + 1
func (pc *PathCalculator) CalculateWorkSegment(hour int) int {
	if pc.config.DayWorkSegments <= 0 {
		return 0 // 0 表示不使用工作段
	}
	if pc.config.DayWorkSegments == 1 {
		return 1
	}

	// 确保小时数在有效范围内
	if hour < 0 {
		hour = 0
	}
	if hour >= 24 {
		hour = 23
	}

	segmentNum := (hour * pc.config.DayWorkSegments / 24) + 1

	// 确保结果在有效范围内
	if segmentNum < 1 {
		segmentNum = 1
	}
	if segmentNum > pc.config.DayWorkSegments {
		segmentNum = pc.config.DayWorkSegments
	}

	return segmentNum
}

// CalculateWeek 计算月内周号（1-based）
// 支持两种模式：
// - calendar: 每7天一周，周号 = floor((日期 - 1) / 7) + 1
// - fixed: 根据配置的周数平均分配，周号 = floor((日期 - 1) * month_weeks / 当月天数) + 1
func (pc *PathCalculator) CalculateWeek(year, month, day int) int {
	// 确保日期在有效范围内
	if day < 1 {
		day = 1
	}

	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := daysInMonth(t)

	if day > daysInMonth {
		day = daysInMonth
	}

	if pc.config.MonthWeeks == "fixed" {
		// fixed 模式：根据配置的周数平均分配
		// 默认使用 5 周（大约每周 6 天）
		fixedWeeks := 5
		weekNum := ((day - 1) * fixedWeeks / daysInMonth) + 1

		if weekNum < 1 {
			weekNum = 1
		}
		if weekNum > fixedWeeks {
			weekNum = fixedWeeks
		}

		return weekNum
	}

	// calendar 模式（默认）：每7天一周
	weekNum := ((day - 1) / 7) + 1

	// 最多5周
	if weekNum > 5 {
		weekNum = 5
	}

	return weekNum
}

// CalculateQuarter 计算年内季度号（1-based）
// 公式：季度号 = floor((月份 - 1) * year_quarters / 12) + 1
func (pc *PathCalculator) CalculateQuarter(month int) int {
	if pc.config.YearQuarters <= 0 {
		return 1
	}
	if pc.config.YearQuarters == 1 {
		return 1
	}

	// 确保月份在有效范围内
	if month < 1 {
		month = 1
	}
	if month > 12 {
		month = 12
	}

	quarterNum := ((month - 1) * pc.config.YearQuarters / 12) + 1

	// 确保结果在有效范围内
	if quarterNum < 1 {
		quarterNum = 1
	}
	if quarterNum > pc.config.YearQuarters {
		quarterNum = pc.config.YearQuarters
	}

	return quarterNum
}

// FormatSegmentDir 格式化分段目录名
// 返回 "S1", "S2", "S3" 等
func (pc *PathCalculator) FormatSegmentDir(segmentNum int) string {
	return fmt.Sprintf("S%d", segmentNum)
}

// FormatWorkSegmentDir 格式化工作段目录名
// 返回 "WS1", "WS2", "WS3" 等
func (pc *PathCalculator) FormatWorkSegmentDir(segmentNum int) string {
	return fmt.Sprintf("WS%d", segmentNum)
}

// FormatWeekDir 格式化周目录名
// 返回 "W1", "W2", "W3" 等
func (pc *PathCalculator) FormatWeekDir(weekNum int) string {
	return fmt.Sprintf("W%d", weekNum)
}

// FormatQuarterDir 格式化季度目录名
// 返回 "Q1", "Q2", "Q3", "Q4" 等
func (pc *PathCalculator) FormatQuarterDir(quarterNum int) string {
	return fmt.Sprintf("Q%d", quarterNum)
}

// daysInMonth 返回指定月份的天数
func daysInMonth(t time.Time) int {
	// 获取下个月的第一天，然后减去一天，得到当月最后一天
	year, month, _ := t.Date()
	nextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	lastDay := nextMonth.Add(-24 * time.Hour)
	return lastDay.Day()
}

// BuildPath 构建完整的层级嵌套路径
// 路径格式：YYYY/QN/MM/WN/DD/WSN/HH/SN/MIN.ext
// 如果某个层级的分段数为1，则跳过该层级
func (pc *PathCalculator) BuildPath(timestamp time.Time, fileType FileType) string {
	year := timestamp.Year()
	month := int(timestamp.Month())
	day := timestamp.Day()
	hour := timestamp.Hour()
	minute := timestamp.Minute()

	// 构建路径组件
	var pathParts []string

	// 1. 年份（YYYY）- 客观周期，始终包含
	pathParts = append(pathParts, fmt.Sprintf("%04d", year))

	// 2. 季度（QN）- 主观周期，如果配置了多个季度则包含
	if pc.config.YearQuarters > 1 {
		quarterNum := pc.CalculateQuarter(month)
		pathParts = append(pathParts, pc.FormatQuarterDir(quarterNum))
	}

	// 根据文件类型决定路径深度
	switch fileType {
	case FileTypeSummaryQuarter:
		// 季度汇总：YYYY/QN/quarter.md
		// 不需要更深的层级

	case FileTypeSummaryMonth:
		// 月汇总：YYYY/QN/MM/month.md
		pathParts = append(pathParts, fmt.Sprintf("%02d", month))

	case FileTypeSummaryWeek:
		// 周汇总：YYYY/QN/MM/WN/week.md
		pathParts = append(pathParts, fmt.Sprintf("%02d", month))
		weekNum := pc.CalculateWeek(year, month, day)
		pathParts = append(pathParts, pc.FormatWeekDir(weekNum))

	case FileTypeSummaryDay:
		// 天汇总：YYYY/QN/MM/WN/DD/day.md
		pathParts = append(pathParts, fmt.Sprintf("%02d", month))
		weekNum := pc.CalculateWeek(year, month, day)
		pathParts = append(pathParts, pc.FormatWeekDir(weekNum))
		pathParts = append(pathParts, fmt.Sprintf("%02d", day))

	case FileTypeSummaryWorkSegment:
		// 工作段汇总：YYYY/QN/MM/WN/DD/WSN/work-segment.md
		pathParts = append(pathParts, fmt.Sprintf("%02d", month))
		weekNum := pc.CalculateWeek(year, month, day)
		pathParts = append(pathParts, pc.FormatWeekDir(weekNum))
		pathParts = append(pathParts, fmt.Sprintf("%02d", day))
		if pc.config.DayWorkSegments > 1 {
			workSegmentNum := pc.CalculateWorkSegment(hour)
			pathParts = append(pathParts, pc.FormatWorkSegmentDir(workSegmentNum))
		}

	case FileTypeSummaryHour:
		// 小时汇总：YYYY/QN/MM/WN/DD/WSN/HH/hour.md
		pathParts = append(pathParts, fmt.Sprintf("%02d", month))
		weekNum := pc.CalculateWeek(year, month, day)
		pathParts = append(pathParts, pc.FormatWeekDir(weekNum))
		pathParts = append(pathParts, fmt.Sprintf("%02d", day))
		if pc.config.DayWorkSegments > 1 {
			workSegmentNum := pc.CalculateWorkSegment(hour)
			pathParts = append(pathParts, pc.FormatWorkSegmentDir(workSegmentNum))
		}
		pathParts = append(pathParts, fmt.Sprintf("%02d", hour))

	case FileTypeSummarySegment, FileTypeScreenshot, FileTypeReport:
		// 分段汇总/截图/报告：YYYY/QN/MM/WN/DD/WSN/HH/SN/file
		pathParts = append(pathParts, fmt.Sprintf("%02d", month))
		weekNum := pc.CalculateWeek(year, month, day)
		pathParts = append(pathParts, pc.FormatWeekDir(weekNum))
		pathParts = append(pathParts, fmt.Sprintf("%02d", day))
		if pc.config.DayWorkSegments > 1 {
			workSegmentNum := pc.CalculateWorkSegment(hour)
			pathParts = append(pathParts, pc.FormatWorkSegmentDir(workSegmentNum))
		}
		pathParts = append(pathParts, fmt.Sprintf("%02d", hour))
		if pc.config.HourSegments > 1 {
			segmentNum := pc.CalculateHourSegment(minute)
			pathParts = append(pathParts, pc.FormatSegmentDir(segmentNum))
		}
	}

	// 9. 文件名
	fileName := pc.getFileName(timestamp, fileType)
	pathParts = append(pathParts, fileName)

	// 组合路径
	return filepath.Join(pathParts...)
}

// getFileName 根据文件类型生成文件名
func (pc *PathCalculator) getFileName(timestamp time.Time, fileType FileType) string {
	minute := timestamp.Minute()

	switch fileType {
	case FileTypeScreenshot:
		return fmt.Sprintf("%02d.png", minute)
	case FileTypeReport:
		return fmt.Sprintf("%02d.md", minute)
	case FileTypeSummarySegment:
		return "summary.md"
	case FileTypeSummaryHour:
		return "hour.md"
	case FileTypeSummaryWorkSegment:
		return "work-segment.md"
	case FileTypeSummaryDay:
		return "day.md"
	case FileTypeSummaryWeek:
		return "week.md"
	case FileTypeSummaryMonth:
		return "month.md"
	case FileTypeSummaryQuarter:
		return "quarter.md"
	default:
		return fmt.Sprintf("%02d.md", minute)
	}
}
