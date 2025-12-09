package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"stuff-time/internal/config"
)

// SummaryLevel 汇总层级
type SummaryLevel int

const (
	SummaryLevelSegment SummaryLevel = iota
	SummaryLevelHour
	SummaryLevelWorkSegment
	SummaryLevelDay
	SummaryLevelWeek
	SummaryLevelMonth
	SummaryLevelQuarter
)

// StorageManager 存储管理器，管理文件的读写操作
type StorageManager struct {
	config         *config.StorageConfig
	pathCalculator *PathCalculator
	basePath       string
}

// NewStorageManager 创建存储管理器
func NewStorageManager(cfg *config.StorageConfig, basePath string) *StorageManager {
	return &StorageManager{
		config:         cfg,
		pathCalculator: NewPathCalculator(cfg),
		basePath:       basePath,
	}
}

// SaveScreenshot 保存截图
func (sm *StorageManager) SaveScreenshot(timestamp time.Time, data []byte) (string, error) {
	if !sm.config.EnableNestedStructure {
		// 如果未启用嵌套结构，使用旧的平铺格式
		return sm.saveLegacyScreenshot(timestamp, data)
	}

	// 构建路径
	relativePath := sm.pathCalculator.BuildPath(timestamp, FileTypeScreenshot)
	fullPath := filepath.Join(sm.basePath, relativePath)

	// 确保目录存在
	if err := sm.ensureDirectory(filepath.Dir(fullPath)); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write screenshot: %w", err)
	}

	return relativePath, nil
}

// SaveReport 保存报告
func (sm *StorageManager) SaveReport(timestamp time.Time, content string) (string, error) {
	if !sm.config.EnableNestedStructure {
		// 如果未启用嵌套结构，使用旧的平铺格式
		return sm.saveLegacyReport(timestamp, content)
	}

	// 构建路径
	relativePath := sm.pathCalculator.BuildPath(timestamp, FileTypeReport)
	fullPath := filepath.Join(sm.basePath, relativePath)

	// 确保目录存在
	if err := sm.ensureDirectory(filepath.Dir(fullPath)); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write report: %w", err)
	}

	return relativePath, nil
}

// SaveSummary 保存汇总
func (sm *StorageManager) SaveSummary(timestamp time.Time, level SummaryLevel, content string) (string, error) {
	if !sm.config.EnableNestedStructure {
		// 如果未启用嵌套结构，使用旧的平铺格式
		return sm.saveLegacySummary(timestamp, level, content)
	}

	// 根据汇总层级选择文件类型
	var fileType FileType
	switch level {
	case SummaryLevelSegment:
		fileType = FileTypeSummarySegment
	case SummaryLevelHour:
		fileType = FileTypeSummaryHour
	case SummaryLevelWorkSegment:
		fileType = FileTypeSummaryWorkSegment
	case SummaryLevelDay:
		fileType = FileTypeSummaryDay
	case SummaryLevelWeek:
		fileType = FileTypeSummaryWeek
	case SummaryLevelMonth:
		fileType = FileTypeSummaryMonth
	case SummaryLevelQuarter:
		fileType = FileTypeSummaryQuarter
	default:
		return "", fmt.Errorf("unsupported summary level: %d", level)
	}

	// 构建路径
	relativePath := sm.pathCalculator.BuildPath(timestamp, fileType)
	fullPath := filepath.Join(sm.basePath, relativePath)

	// 确保目录存在
	if err := sm.ensureDirectory(filepath.Dir(fullPath)); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write summary: %w", err)
	}

	return relativePath, nil
}

// ensureDirectory 确保目录存在
func (sm *StorageManager) ensureDirectory(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// saveLegacyScreenshot 使用旧格式保存截图
func (sm *StorageManager) saveLegacyScreenshot(timestamp time.Time, data []byte) (string, error) {
	// 旧格式：YYYY/MM/DD/HH/MIN-SEC.png
	year := timestamp.Year()
	month := int(timestamp.Month())
	day := timestamp.Day()
	hour := timestamp.Hour()
	minute := timestamp.Minute()
	second := timestamp.Second()

	dirPath := filepath.Join(
		sm.basePath,
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
		fmt.Sprintf("%02d", day),
		fmt.Sprintf("%02d", hour),
	)

	if err := sm.ensureDirectory(dirPath); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	filename := fmt.Sprintf("%02d-%02d.png", minute, second)
	fullPath := filepath.Join(dirPath, filename)

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write screenshot: %w", err)
	}

	relativePath := filepath.Join(
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
		fmt.Sprintf("%02d", day),
		fmt.Sprintf("%02d", hour),
		filename,
	)

	return relativePath, nil
}

// saveLegacyReport 使用旧格式保存报告
func (sm *StorageManager) saveLegacyReport(timestamp time.Time, content string) (string, error) {
	// 旧格式：YYYY/MM/DD/HH/MIN-SEC.md
	year := timestamp.Year()
	month := int(timestamp.Month())
	day := timestamp.Day()
	hour := timestamp.Hour()
	minute := timestamp.Minute()
	second := timestamp.Second()

	dirPath := filepath.Join(
		sm.basePath,
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
		fmt.Sprintf("%02d", day),
		fmt.Sprintf("%02d", hour),
	)

	if err := sm.ensureDirectory(dirPath); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	filename := fmt.Sprintf("%02d-%02d.md", minute, second)
	fullPath := filepath.Join(dirPath, filename)

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write report: %w", err)
	}

	relativePath := filepath.Join(
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", month),
		fmt.Sprintf("%02d", day),
		fmt.Sprintf("%02d", hour),
		filename,
	)

	return relativePath, nil
}

// saveLegacySummary 使用旧格式保存汇总
func (sm *StorageManager) saveLegacySummary(timestamp time.Time, level SummaryLevel, content string) (string, error) {
	year := timestamp.Year()
	month := int(timestamp.Month())
	day := timestamp.Day()
	hour := timestamp.Hour()

	var dirPath string
	var filename string

	switch level {
	case SummaryLevelSegment, SummaryLevelHour:
		// 小时内汇总和小时汇总都在小时目录下
		dirPath = filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
			fmt.Sprintf("%02d", hour),
		)
		if level == SummaryLevelSegment {
			minute := timestamp.Minute()
			filename = fmt.Sprintf("fifteenmin-%02d.md", minute)
		} else {
			filename = "hour.md"
		}

	case SummaryLevelWorkSegment, SummaryLevelDay:
		// 工作段和天汇总在日目录下
		dirPath = filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
		)
		if level == SummaryLevelWorkSegment {
			workSegment := sm.pathCalculator.CalculateWorkSegment(hour)
			filename = fmt.Sprintf("work-segment-%d.md", workSegment)
		} else {
			filename = "day.md"
		}

	case SummaryLevelWeek:
		// 周汇总在月目录下
		dirPath = filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
		)
		_, week := timestamp.ISOWeek()
		filename = fmt.Sprintf("week-W%02d.md", week)

	case SummaryLevelMonth:
		// 月汇总在月目录下
		dirPath = filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
		)
		filename = "month.md"

	case SummaryLevelQuarter:
		// 季度汇总在年目录下
		dirPath = filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
		)
		quarter := (month-1)/3 + 1
		filename = fmt.Sprintf("quarter-Q%d.md", quarter)

	default:
		return "", fmt.Errorf("unsupported summary level: %d", level)
	}

	if err := sm.ensureDirectory(dirPath); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath := filepath.Join(dirPath, filename)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write summary: %w", err)
	}

	// 计算相对路径
	relPath, err := filepath.Rel(sm.basePath, fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate relative path: %w", err)
	}

	return relPath, nil
}

// GetFile 获取文件（支持向后兼容并自动规范化）
func (sm *StorageManager) GetFile(timestamp time.Time, fileType FileType) (string, error) {
	// 首先尝试新格式路径
	if sm.config.EnableNestedStructure {
		relativePath := sm.pathCalculator.BuildPath(timestamp, fileType)
		fullPath := filepath.Join(sm.basePath, relativePath)

		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	// 如果启用了向后兼容模式，尝试旧格式路径
	if sm.config.BackwardCompatible {
		legacyPath, err := sm.tryLegacyPath(timestamp, fileType)
		if err == nil {
			// 找到旧格式文件，自动规范化为新格式
			if sm.config.EnableNestedStructure {
				newPath, normalizeErr := sm.normalizeFile(legacyPath, timestamp, fileType)
				if normalizeErr == nil {
					// 规范化成功，返回新路径
					return newPath, nil
				}
				// 规范化失败，仍然返回旧路径（但记录警告）
				// 这样至少不会影响文件访问
			}
			return legacyPath, nil
		}
	}

	return "", fmt.Errorf("file not found for timestamp %v and type %d", timestamp, fileType)
}

// normalizeFile 将旧格式文件规范化为新格式
// 返回新文件的完整路径
func (sm *StorageManager) normalizeFile(oldPath string, timestamp time.Time, fileType FileType) (string, error) {
	// 读取旧文件内容
	content, err := os.ReadFile(oldPath)
	if err != nil {
		return "", fmt.Errorf("failed to read old file: %w", err)
	}

	// 构建新路径
	relativePath := sm.pathCalculator.BuildPath(timestamp, fileType)
	newPath := filepath.Join(sm.basePath, relativePath)

	// 确保新目录存在
	if err := sm.ensureDirectory(filepath.Dir(newPath)); err != nil {
		return "", fmt.Errorf("failed to create new directory: %w", err)
	}

	// 写入新文件
	if err := os.WriteFile(newPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write new file: %w", err)
	}

	// 删除旧文件
	if err := os.Remove(oldPath); err != nil {
		// 删除失败不是致命错误，新文件已经创建成功
		// 可以记录日志，但不返回错误
		_ = err
	}

	return newPath, nil
}

// tryLegacyPath 尝试旧格式路径
func (sm *StorageManager) tryLegacyPath(timestamp time.Time, fileType FileType) (string, error) {
	year := timestamp.Year()
	month := int(timestamp.Month())
	day := timestamp.Day()
	hour := timestamp.Hour()
	minute := timestamp.Minute()

	var possiblePaths []string

	switch fileType {
	case FileTypeScreenshot:
		// 尝试 MIN-SEC.png 格式（遍历所有可能的秒数）
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
			fmt.Sprintf("%02d", hour),
		)

		// 尝试 MIN.png 格式（新格式但在旧目录结构中）
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("%02d.png", minute)))

		// 尝试 MIN-SEC.png 格式（旧格式）
		for sec := 0; sec < 60; sec++ {
			possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("%02d-%02d.png", minute, sec)))
		}

	case FileTypeReport:
		// 尝试 MIN-SEC.md 格式
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
			fmt.Sprintf("%02d", hour),
		)

		// 尝试 MIN.md 格式（新格式但在旧目录结构中）
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("%02d.md", minute)))

		// 尝试 MIN-SEC.md 格式（旧格式）
		for sec := 0; sec < 60; sec++ {
			possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("%02d-%02d.md", minute, sec)))
		}

	case FileTypeSummarySegment:
		// 尝试旧的 fifteenmin-XX.md 格式
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
			fmt.Sprintf("%02d", hour),
		)
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("fifteenmin-%02d.md", minute)))
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, "summary.md"))

	case FileTypeSummaryHour:
		// 小时汇总
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
			fmt.Sprintf("%02d", hour),
		)
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, "hour.md"))

	case FileTypeSummaryDay:
		// 天汇总
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
			fmt.Sprintf("%02d", day),
		)
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, "day.md"))

	case FileTypeSummaryWeek:
		// 周汇总
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
		)
		_, week := timestamp.ISOWeek()
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("week-W%02d.md", week)))

	case FileTypeSummaryMonth:
		// 月汇总
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", month),
		)
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, "month.md"))

	case FileTypeSummaryQuarter:
		// 季度汇总
		baseDir := filepath.Join(
			sm.basePath,
			fmt.Sprintf("%04d", year),
		)
		quarter := (month-1)/3 + 1
		possiblePaths = append(possiblePaths, filepath.Join(baseDir, fmt.Sprintf("quarter-Q%d.md", quarter)))
	}

	// 尝试所有可能的路径
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no legacy path found")
}

// IsLegacyFormat 检查文件名是否为旧格式（MIN-SEC 格式）
func IsLegacyFormat(filename string) bool {
	// 检查是否匹配 XX-XX.ext 格式
	if len(filename) < 8 {
		return false
	}

	// 检查是否有连字符
	if filename[2] != '-' {
		return false
	}

	// 检查扩展名
	ext := filepath.Ext(filename)
	if ext != ".png" && ext != ".md" {
		return false
	}

	// 检查是否为数字-数字格式
	base := filename[:len(filename)-len(ext)]
	if len(base) != 5 { // XX-XX
		return false
	}

	// 简单验证：第0-1位和第3-4位应该是数字
	for i, c := range base {
		if i == 2 {
			continue // 跳过连字符
		}
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}
