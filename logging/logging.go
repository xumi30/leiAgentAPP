package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
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
	name        string
	level       LogLevel
	logChan     chan string
	file        *os.File
	filePath    string
	maxSize     int64
	currentSize int64
	wg          sync.WaitGroup
	once        sync.Once
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
	defaultLogger = NewLogger("default", "logs/default.log", INFO, 10*1024*1024)
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

	// 创建新的 Logger 实例
	logger = &Logger{
		name:     name,
		level:    level,
		logChan:  make(chan string, 1000), // 缓冲通道大小为1000
		filePath: filePath,
		maxSize:  maxSize,
	}

	// 确保日志目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("创建日志目录失败: %v", err))
	}

	// 打开或创建日志文件
	if err := logger.openFile(); err != nil {
		panic(fmt.Sprintf("打开日志文件失败: %v", err))
	}

	// 启动日志写入协程
	logger.wg.Add(1)
	go logger.writeLog()

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

	// 获取当前文件大小
	info, err := l.file.Stat()
	if err != nil {
		return err
	}
	l.currentSize = info.Size()

	return nil
}

// rotateFile 日志文件轮转
func (l *Logger) rotateFile() error {
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

// writeLog 日志写入协程
func (l *Logger) writeLog() {
	defer l.wg.Done()

	for log := range l.logChan {
		// 检查是否需要轮转
		if l.maxSize > 0 && l.currentSize >= l.maxSize {
			if err := l.rotateFile(); err != nil {
				fmt.Printf("日志文件轮转失败: %v\n", err)
			}
		}

		// 写入日志
		n, err := fmt.Fprintln(l.file, log)
		if err != nil {
			fmt.Printf("写入日志失败: %v\n", err)
			continue
		}
		l.currentSize += int64(n)
	}
}

// formatLog 格式化日志
func (l *Logger) formatLog(level LogLevel, format string, v ...interface{}) string {
	levelStr := ""
	switch level {
	case DEBUG:
		levelStr = "DEBUG"
	case INFO:
		levelStr = "INFO"
	case WARN:
		levelStr = "WARN"
	case ERROR:
		levelStr = "ERROR"
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	return fmt.Sprintf("[%s] [%s] [%s] %s", timestamp, l.name, levelStr, fmt.Sprintf(format, v...))
}

// formatLogWithLocation 带位置的格式化日志
func (l *Logger) formatLogWithLocation(level LogLevel, file string, line int, format string, v ...interface{}) string {
	levelStr := ""
	switch level {
	case DEBUG:
		levelStr = "DEBUG"
	case INFO:
		levelStr = "INFO"
	case WARN:
		levelStr = "WARN"
	case ERROR:
		levelStr = "ERROR"
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	return fmt.Sprintf("[%s] [%s] [%s] [%s:%d] %s", timestamp, l.name, levelStr, filepath.Base(file), line, fmt.Sprintf(format, v...))
}

// log 内部日志方法
func (l *Logger) log(level LogLevel, format string, v ...interface{}) {
	if level < l.level {
		return
	}

	log := l.formatLog(level, format, v...)
	l.logChan <- log
}

// logWithLocation 带位置的内部日志方法
func (l *Logger) logWithLocation(level LogLevel, file string, line int, format string, v ...interface{}) {
	if level < l.level {
		return
	}

	log := l.formatLogWithLocation(level, file, line, format, v...)
	l.logChan <- log
}

// Debug 输出DEBUG级别日志
func (l *Logger) Debug(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	l.logWithLocation(DEBUG, file, line, format, v...)
}

// Info 输出INFO级别日志
func (l *Logger) Info(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	l.logWithLocation(INFO, file, line, format, v...)
}

// Warn 输出WARN级别日志
func (l *Logger) Warn(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	l.logWithLocation(WARN, file, line, format, v...)
}

// Error 输出ERROR级别日志
func (l *Logger) Error(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	l.logWithLocation(ERROR, file, line, format, v...)
}

// Close 关闭日志
func (l *Logger) Close() error {
	l.once.Do(func() {
		close(l.logChan)
		l.wg.Wait()
		if l.file != nil {
			l.file.Close()
		}
	})
	return nil
}

// 以下为包级别的便捷函数，使用默认 logger

// Debug 输出DEBUG级别日志（使用默认 logger）
func Debug(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	defaultLogger.logWithLocation(DEBUG, file, line, format, v...)
}

// Info 输出INFO级别日志（使用默认 logger）
func Info(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	defaultLogger.logWithLocation(INFO, file, line, format, v...)
}

// Warn 输出WARN级别日志（使用默认 logger）
func Warn(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	defaultLogger.logWithLocation(WARN, file, line, format, v...)
}

// Error 输出ERROR级别日志（使用默认 logger）
func Error(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	defaultLogger.logWithLocation(ERROR, file, line, format, v...)
}
