package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LogLevel 日志级别类型
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger 异步日志结构体
type Logger struct {
	name     string
	level    LogLevel
	logger   zerolog.Logger
	file     *os.File
	filePath string
	maxSize  int64
	mu       sync.Mutex
	once     sync.Once
}

// loggers 存储已创建的 Logger 实例
var (
	loggers = make(map[string]*Logger)
	mu      sync.RWMutex
)

// defaultLogger 默认的 logger 实例
var defaultLogger *Logger

// init 初始化默认 logger
func init() {
	// 设置全局时间格式为 RFC3339
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	// 设置全局日志级别为 Info
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// 设置控制台输出格式
	consoleWriter := &k8sConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000Z07:00",
		NoColor:    false,
	}

	// 创建默认 logger
	defaultLogger = NewLogger("default", "logs/default.log", INFO, 10*1024*1024)

	// 设置全局日志输出
	log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()
}

// NewLogger 创建新的日志实例
func NewLogger(name, filePath string, level LogLevel, maxSize int64) *Logger {
	key := name

	// 先尝试获取读锁，检查是否已存在
	mu.RLock()
	logger, exists := loggers[key]
	mu.RUnlock()

	if exists {
		return logger
	}

	// 获取写锁，创建新实例
	mu.Lock()
	defer mu.Unlock()

	// 再次检查，防止在获取写锁期间其他 goroutine 已经创建了实例
	if logger, exists := loggers[key]; exists {
		return logger
	}

	// 确保日志目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("创建日志目录失败: %v", err))
	}

	// 打开或创建日志文件
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(fmt.Sprintf("打开日志文件失败: %v", err))
	}

	// 创建文件写入器，设置格式
	fileWriter := &k8sConsoleWriter{
		Out:        file,
		TimeFormat: "2006-01-02T15:04:05.000Z07:00",
		NoColor:    true,
	}

	// 创建新的 Logger 实例
	zl := zerolog.New(fileWriter).With().Timestamp().Str("component", name).Logger()
	zl = zl.Level(convertLevel(level))

	logger = &Logger{
		name:     name,
		level:    level,
		logger:   zl,
		file:     file,
		filePath: filePath,
		maxSize:  maxSize,
	}

	// 存储新创建的 Logger 实例
	loggers[key] = logger

	return logger
}

// GetLogger 根据名称获取 Logger 实例
func GetLogger(name string) (*Logger, error) {
	mu.RLock()
	defer mu.RUnlock()

	logger, exists := loggers[name]
	if !exists {
		return nil, fmt.Errorf("logger '%s' not found", name)
	}
	return logger, nil
}

// SetDefaultLogger 设置默认的 logger 实例
func SetDefaultLogger(logger *Logger) {
	mu.Lock()
	defer mu.Unlock()
	defaultLogger = logger
}

// GetDefaultLogger 获取默认的 logger 实例
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// openFile 打开日志文件
func (l *Logger) openFile() error {
	var err error
	l.file, err = os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// 创建文件写入器，设置格式
	fileWriter := &k8sConsoleWriter{
		Out:        l.file,
		TimeFormat: "2006-01-02T15:04:05.000Z07:00",
		NoColor:    true,
	}

	// 更新 zerolog logger 的输出
	l.logger = zerolog.New(fileWriter).With().Timestamp().Str("component", l.name).Logger()
	l.logger = l.logger.Level(convertLevel(l.level))

	return nil
}

// rotateFile 日志文件轮转
func (l *Logger) rotateFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
	}

	// 重命名当前日志文件
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := fmt.Sprintf("%s.%s", l.filePath, timestamp)
	if err := os.Rename(l.filePath, rotatedPath); err != nil {
		return err
	}

	// 创建新的日志文件
	return l.openFile()
}

// checkRotation 检查是否需要轮转
func (l *Logger) checkRotation() error {
	if l.maxSize <= 0 {
		return nil
	}

	info, err := l.file.Stat()
	if err != nil {
		return err
	}

	if info.Size() >= l.maxSize {
		return l.rotateFile()
	}

	return nil
}

// Debug 输出DEBUG级别日志
func (l *Logger) Debug(format string, v ...interface{}) {
	l.checkRotation()
	l.logger.Debug().Caller(2).Msgf(format, v...)
}

// Info 输出INFO级别日志
func (l *Logger) Info(format string, v ...interface{}) {
	l.checkRotation()
	l.logger.Info().Caller(2).Msgf(format, v...)
}

// Warn 输出WARN级别日志
func (l *Logger) Warn(format string, v ...interface{}) {
	l.checkRotation()
	l.logger.Warn().Caller(2).Msgf(format, v...)
}

// Error 输出ERROR级别日志
func (l *Logger) Error(format string, v ...interface{}) {
	l.checkRotation()
	l.logger.Error().Caller(2).Msgf(format, v...)
}

// Close 关闭日志
func (l *Logger) Close() error {
	l.once.Do(func() {
		if l.file != nil {
			l.file.Close()
		}
	})
	return nil
}

// 以下为包级别的便捷函数，使用默认 logger

// Debug 输出DEBUG级别日志（使用默认 logger）
func Debug(format string, v ...interface{}) {
	defaultLogger.Debug(format, v...)
}

// Info 输出INFO级别日志（使用默认 logger）
func Info(format string, v ...interface{}) {
	defaultLogger.Info(format, v...)
}

// Warn 输出WARN级别日志（使用默认 logger）
func Warn(format string, v ...interface{}) {
	defaultLogger.Warn(format, v...)
}

// Error 输出ERROR级别日志（使用默认 logger）
func Error(format string, v ...interface{}) {
	defaultLogger.Error(format, v...)
}

// convertLevel 将自定义日志级别转换为zerolog级别
func convertLevel(level LogLevel) zerolog.Level {
	switch level {
	case DEBUG:
		return zerolog.DebugLevel
	case INFO:
		return zerolog.InfoLevel
	case WARN:
		return zerolog.WarnLevel
	case ERROR:
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// k8sConsoleWriter 自定义控制台写入器，实现Kubernetes风格的日志输出
type k8sConsoleWriter struct {
	Out        io.Writer
	TimeFormat string
	NoColor    bool
}

// Write 实现io.Writer接口
func (w *k8sConsoleWriter) Write(p []byte) (n int, err error) {
	// 解析日志事件
	var event map[string]interface{}
	if err := json.Unmarshal(p, &event); err != nil {
		return w.Out.Write(p)
	}

	// 构建日志输出
	var builder strings.Builder

	// 添加时间戳
	if timestamp, ok := event[zerolog.TimestampFieldName].(string); ok {
		builder.WriteString(timestamp)
		builder.WriteString(" ")
	}

	// 添加日志级别
	if level, ok := event[zerolog.LevelFieldName].(string); ok {
		levelStr := strings.ToUpper(level)
		if !w.NoColor {
			switch levelStr {
			case "DEBUG":
				builder.WriteString("\x1b[36m") // 青色
			case "INFO":
				builder.WriteString("\x1b[32m") // 绿色
			case "WARN":
				builder.WriteString("\x1b[33m") // 黄色
			case "ERROR":
				builder.WriteString("\x1b[31m") // 红色
			}
		}
		builder.WriteString(levelStr)
		if !w.NoColor {
			builder.WriteString("\x1b[0m") // 重置颜色
		}
		builder.WriteString(" ")
	}

	// 添加组件名称
	if component, ok := event["component"].(string); ok {
		builder.WriteString("[")
		builder.WriteString(component)
		builder.WriteString("] ")
	}

	// 添加调用位置
	if caller, ok := event[zerolog.CallerFieldName].(string); ok {
		builder.WriteString("(")
		builder.WriteString(caller)
		builder.WriteString(") ")
	}

	// 添加消息
	if message, ok := event[zerolog.MessageFieldName].(string); ok {
		builder.WriteString(message)
	}

	// 添加换行
	builder.WriteString("\n")

	return w.Out.Write([]byte(builder.String()))
}
