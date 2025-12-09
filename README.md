Stuff time 是一个用于记录 stuff 工作日常的agent，主要思路是通过定时截屏，收集工作桌面的信息，通过LLM 分析总结，以小时为单位进行时间管理。

## 构建说明

### 前置要求

- Go 1.25.0 或更高版本
- macOS 15.6.1 或更高版本（用于截屏功能）
- 已配置的 OpenAI API 密钥

### 快速开始

使用 Makefile 构建（推荐）：

```bash
# 查看所有可用命令
make help

# 开发构建
make build

# 发布构建（优化）
make build-release

# 安装到用户目录（~/bin）
make install-user

# 安装到系统目录（/usr/local/bin，需要 sudo）
make install
```

### 手动构建

```bash
# 下载依赖
go mod download

# 构建二进制文件
go build -o stuff-time ./cmd/stuff-time

# 运行
./stuff-time --help
```

### 跨平台构建

```bash
# 构建 macOS 版本（arm64 和 amd64）
make build-darwin

# 构建 Linux 版本
make build-linux

# 构建 Windows 版本
make build-windows

# 构建所有平台
make build-all
```

构建产物会保存在 `build/` 目录中。

### 其他常用命令

```bash
# 运行测试
make test

# 运行测试并生成覆盖率报告
make test-coverage

# 格式化代码
make fmt

# 清理构建产物
make clean

# 清理所有（包括数据文件）
make clean-all
```

## 工作方式

- **分钟级截屏**：每分钟自动截取鼠标所在屏幕的截图
- **批量分析**：定时批量分析未分析的截图（默认每30分钟），避免频繁调用API
- **多维度总结**：同时支持多个时间维度的总结（默认：半小时、每天、每周、每月），帮助发现提效机会
- **累计总结查询**：支持按天、周、月、年查看累计总结

## 平台要求

- macOS 15.6.1 (24G90) 以及以上
- 截屏要求：鼠标或者光标所在的屏幕（多屏幕时）

## 配置说明

### OpenAI 配置

- `openai.api_key`: OpenAI API 密钥
- `openai.model`: 使用的模型（默认：gpt-4-vision-preview）
- `openai.max_tokens`: 最大 token 数（默认：5000）
- `openai.prompt`: **截图分析提示词**（信息提取）
  - 位置：`config/config.yaml` 中的 `openai.prompt`
  - 用途：分析单张截图，提取用户活动信息
  - 格式：中文，包含【摘要】和【详细论述】
- `openai.behavior_analysis_prompt`: **周期总结提示词**（行为分析与提效建议）
  - 位置：`config/config.yaml` 中的 `openai.behavior_analysis_prompt`
  - 用途：基于时间段内的活动信息进行行为分析和提效建议
  - 格式：中文，包含【行为分析】和【提效建议】
  - **这是 summary 报告使用的提示词，可在配置文件中修改**

### 截图配置

- `screenshot.interval`: 截屏间隔（默认1分钟）
- `screenshot.analysis_interval`: 批量分析间隔（默认10分钟）
- `screenshot.analysis_workers`: 并发分析工作线程数（默认3个）
  - 使用 worker pool 模式并发分析多张截图，提升分析效率
  - 可根据 API 限制和系统资源调整，建议范围：1-5
- `screenshot.summary_periods`: 总结周期列表（支持：halfhour, hour, day, week, month, year）
  - 默认：`["halfhour", "day", "week", "month"]`
  - 可以同时配置多个周期，系统会为每个周期自动生成总结
- 支持 cron 表达式或 fixed rate 两种定时方式

## 命令说明

### 用户命令

- `start`: 启动定时截屏和分析任务（前台运行）
- `daemon start`: 以后台守护进程方式启动
- `daemon stop`: 停止后台守护进程
- `daemon restart`: 重启后台守护进程
- `daemon status`: 查看守护进程状态
- `generate`: 生成周期总结报告
  - `--period` / `-p`: 指定周期类型（hour, day, week, month, year），默认 `day`
  - `--date` / `-d`: 指定报告日期（格式：2006-01-02），默认为当前日期
- `status`: 查看当前状态和统计
- `query`: 查询已完成的历史报告（按小时/日期）
  - **强调过去已完成**：查询已经生成的完整周期报告
  - `--date`: 指定日期（YYYY-MM-DD）
  - `--hour`: 指定小时（0-23）
- `summary`: 查看累计总结（按天/周/月/年）
- `config`: 显示当前配置
- `cleanup`: 清理旧数据

### 调试命令

- `trigger`: 手动触发调试操作（**仅用于开发和调试，完全独立，不依赖 daemon**）
  - `--screenshot`: 手动截屏
  - `--analyze`: 手动批量分析
  - `--all`: 执行所有调试操作（截屏+分析）
  - `--verbose` / `-v`: 启用详细输出模式，用于问题排查

## 后台运行

使用 `daemon` 命令可以以后台方式运行：

```bash
# 启动后台守护进程
./stuff-time daemon start

# 查看守护进程状态
./stuff-time daemon status

# 停止守护进程
./stuff-time daemon stop

# 重启守护进程
./stuff-time daemon restart
```

守护进程的日志文件默认保存在 `~/.stuff-time.log`，PID文件保存在 `~/.stuff-time.pid`。