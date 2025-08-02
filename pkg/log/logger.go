package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = map[LogLevel]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
}

type Logger struct {
	level  LogLevel
	logger *log.Logger
}

func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:  level,
		logger: log.New(os.Stdout, "", 0),
	}
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// Debug 记录调试信息
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info 记录信息
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn 记录警告信息
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error 记录错误信息
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Fatal 记录致命错误并退出
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LevelFatal, format, args...)
	os.Exit(1)
}

// log 内部日志记录方法
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	// 获取调用信息
	_, file, line, ok := runtime.Caller(2)
	fileName := "unknown"
	if ok {
		fileName = filepath.Base(file)
	}

	// 格式化时间
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// 格式化消息
	message := fmt.Sprintf(format, args...)

	// 构建日志格式
	logEntry := fmt.Sprintf("[%s] [%s] [%s:%d] %s",
		timestamp,
		levelNames[level],
		fileName,
		line,
		message)

	// 输出日志
	l.logger.Println(logEntry)
}

// FileLogger 是文件日志记录器
type FileLogger struct {
	*Logger
	file *os.File
}

// NewFileLogger 创建新的文件日志记录器
func NewFileLogger(logFile string, level LogLevel) (*FileLogger, error) {
	// 确保日志目录存在
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 打开或创建日志文件
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	logger := NewLogger(level)
	logger.logger = log.New(file, "", 0)

	return &FileLogger{
		Logger: logger,
		file:   file,
	}, nil
}

// Close 关闭日志文件
func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Global logger instance
var globalLogger *Logger

// InitLogger 初始化全局日志记录器
func InitLogger(level LogLevel) {
	globalLogger = NewLogger(level)
}

// GetLogger 获取全局日志记录器
func GetLogger() *Logger {
	if globalLogger == nil {
		globalLogger = NewLogger(LevelInfo)
	}
	return globalLogger
}

// Convenience functions
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

func Fatal(format string, args ...interface{}) {
	GetLogger().Fatal(format, args...)
}
