package logging

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tis24dev/proxmox-backup/internal/types"
)

// Logger gestisce il logging dell'applicazione
type Logger struct {
	level      types.LogLevel
	useColor   bool
	output     io.Writer
	timeFormat string
}

// New crea un nuovo logger
func New(level types.LogLevel, useColor bool) *Logger {
	return &Logger{
		level:      level,
		useColor:   useColor,
		output:     os.Stdout,
		timeFormat: "2006-01-02 15:04:05",
	}
}

// SetOutput imposta l'output writer del logger
func (l *Logger) SetOutput(w io.Writer) {
	l.output = w
}

// SetLevel imposta il livello di logging
func (l *Logger) SetLevel(level types.LogLevel) {
	l.level = level
}

// GetLevel restituisce il livello corrente
func (l *Logger) GetLevel() types.LogLevel {
	return l.level
}

// log è il metodo interno per scrivere i log
func (l *Logger) log(level types.LogLevel, format string, args ...interface{}) {
	if level > l.level {
		return
	}

	timestamp := time.Now().Format(l.timeFormat)
	levelStr := level.String()
	message := fmt.Sprintf(format, args...)

	var colorCode string
	var resetCode string

	if l.useColor {
		resetCode = "\033[0m"
		switch level {
		case types.LogLevelDebug:
			colorCode = "\033[36m" // Cyan
		case types.LogLevelInfo:
			colorCode = "\033[32m" // Green
		case types.LogLevelWarning:
			colorCode = "\033[33m" // Yellow
		case types.LogLevelError:
			colorCode = "\033[31m" // Red
		case types.LogLevelCritical:
			colorCode = "\033[1;31m" // Bold Red
		}
	}

	output := fmt.Sprintf("[%s] %s%-8s%s %s\n",
		timestamp,
		colorCode,
		levelStr,
		resetCode,
		message,
	)

	fmt.Fprint(l.output, output)
}

// Debug scrive un log di debug
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(types.LogLevelDebug, format, args...)
}

// Info scrive un log informativo
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(types.LogLevelInfo, format, args...)
}

// Warning scrive un log di warning
func (l *Logger) Warning(format string, args ...interface{}) {
	l.log(types.LogLevelWarning, format, args...)
}

// Error scrive un log di errore
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(types.LogLevelError, format, args...)
}

// Critical scrive un log critico
func (l *Logger) Critical(format string, args ...interface{}) {
	l.log(types.LogLevelCritical, format, args...)
}

// Fatal scrive un log critico ed esce con il codice specificato
func (l *Logger) Fatal(exitCode types.ExitCode, format string, args ...interface{}) {
	l.Critical(format, args...)
	os.Exit(exitCode.Int())
}

// Package-level default logger
var defaultLogger *Logger

func init() {
	defaultLogger = New(types.LogLevelInfo, true)
}

// SetDefaultLogger imposta il logger di default
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// GetDefaultLogger restituisce il logger di default
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// Package-level convenience functions

// Debug scrive un log di debug usando il logger di default
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

// Info scrive un log informativo usando il logger di default
func Info(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

// Warning scrive un log di warning usando il logger di default
func Warning(format string, args ...interface{}) {
	defaultLogger.Warning(format, args...)
}

// Error scrive un log di errore usando il logger di default
func Error(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}

// Critical scrive un log critico usando il logger di default
func Critical(format string, args ...interface{}) {
	defaultLogger.Critical(format, args...)
}

// Fatal scrive un log critico ed esce con il codice specificato
func Fatal(exitCode types.ExitCode, format string, args ...interface{}) {
	defaultLogger.Fatal(exitCode, format, args...)
}
