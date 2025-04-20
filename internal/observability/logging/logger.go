// Package logging provides logging utilities for the application.
package logging

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger is the interface for logging.
type Logger interface {
	// Debug logs a debug message.
	Debug(args ...interface{})
	// Debugf logs a formatted debug message.
	Debugf(format string, args ...interface{})
	// Info logs an info message.
	Info(args ...interface{})
	// Infof logs a formatted info message.
	Infof(format string, args ...interface{})
	// Warn logs a warning message.
	Warn(args ...interface{})
	// Warnf logs a formatted warning message.
	Warnf(format string, args ...interface{})
	// Error logs an error message.
	Error(args ...interface{})
	// Errorf logs a formatted error message.
	Errorf(format string, args ...interface{})
	// Fatal logs a fatal message and exits.
	Fatal(args ...interface{})
	// Fatalf logs a formatted fatal message and exits.
	Fatalf(format string, args ...interface{})
	// WithField adds a field to the logger.
	WithField(key string, value interface{}) Logger
	// WithFields adds multiple fields to the logger.
	WithFields(fields map[string]interface{}) Logger
	// WithError adds an error to the logger.
	WithError(err error) Logger
}

// LogrusLogger is an implementation of the Logger interface using Logrus.
type LogrusLogger struct {
	logger *logrus.Entry
}

// NewLogrusLogger creates a new LogrusLogger.
func NewLogrusLogger() *LogrusLogger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.999Z07:00",
	})
	return &LogrusLogger{
		logger: logrus.NewEntry(logger),
	}
}

// Debug logs a debug message.
func (l *LogrusLogger) Debug(args ...interface{}) {
	l.logger.Debug(args...)
}

// Debugf logs a formatted debug message.
func (l *LogrusLogger) Debugf(format string, args ...interface{}) {
	l.logger.Debugf(format, args...)
}

// Info logs an info message.
func (l *LogrusLogger) Info(args ...interface{}) {
	l.logger.Info(args...)
}

// Infof logs a formatted info message.
func (l *LogrusLogger) Infof(format string, args ...interface{}) {
	l.logger.Infof(format, args...)
}

// Warn logs a warning message.
func (l *LogrusLogger) Warn(args ...interface{}) {
	l.logger.Warn(args...)
}

// Warnf logs a formatted warning message.
func (l *LogrusLogger) Warnf(format string, args ...interface{}) {
	l.logger.Warnf(format, args...)
}

// Error logs an error message.
func (l *LogrusLogger) Error(args ...interface{}) {
	l.logger.Error(args...)
}

// Errorf logs a formatted error message.
func (l *LogrusLogger) Errorf(format string, args ...interface{}) {
	l.logger.Errorf(format, args...)
}

// Fatal logs a fatal message and exits.
func (l *LogrusLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits.
func (l *LogrusLogger) Fatalf(format string, args ...interface{}) {
	l.logger.Fatalf(format, args...)
}

// WithField adds a field to the logger.
func (l *LogrusLogger) WithField(key string, value interface{}) Logger {
	return &LogrusLogger{
		logger: l.logger.WithField(key, value),
	}
}

// WithFields adds multiple fields to the logger.
func (l *LogrusLogger) WithFields(fields map[string]interface{}) Logger {
	return &LogrusLogger{
		logger: l.logger.WithFields(logrus.Fields(fields)),
	}
}

// WithError adds an error to the logger.
func (l *LogrusLogger) WithError(err error) Logger {
	return &LogrusLogger{
		logger: l.logger.WithError(err),
	}
}

// SetLevel sets the logging level.
func (l *LogrusLogger) SetLevel(level string) {
	switch level {
	case "debug":
		l.logger.Logger.SetLevel(logrus.DebugLevel)
	case "info":
		l.logger.Logger.SetLevel(logrus.InfoLevel)
	case "warn":
		l.logger.Logger.SetLevel(logrus.WarnLevel)
	case "error":
		l.logger.Logger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		l.logger.Logger.SetLevel(logrus.FatalLevel)
	default:
		l.logger.Logger.SetLevel(logrus.InfoLevel)
	}
}

// DefaultLogger is the default logger for the application.
var DefaultLogger Logger = NewLogrusLogger()

// Debug logs a debug message.
func Debug(args ...interface{}) {
	DefaultLogger.Debug(args...)
}

// Debugf logs a formatted debug message.
func Debugf(format string, args ...interface{}) {
	DefaultLogger.Debugf(format, args...)
}

// Info logs an info message.
func Info(args ...interface{}) {
	DefaultLogger.Info(args...)
}

// Infof logs a formatted info message.
func Infof(format string, args ...interface{}) {
	DefaultLogger.Infof(format, args...)
}

// Warn logs a warning message.
func Warn(args ...interface{}) {
	DefaultLogger.Warn(args...)
}

// Warnf logs a formatted warning message.
func Warnf(format string, args ...interface{}) {
	DefaultLogger.Warnf(format, args...)
}

// Error logs an error message.
func Error(args ...interface{}) {
	DefaultLogger.Error(args...)
}

// Errorf logs a formatted error message.
func Errorf(format string, args ...interface{}) {
	DefaultLogger.Errorf(format, args...)
}

// Fatal logs a fatal message and exits.
func Fatal(args ...interface{}) {
	DefaultLogger.Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits.
func Fatalf(format string, args ...interface{}) {
	DefaultLogger.Fatalf(format, args...)
}

// WithField adds a field to the logger.
func WithField(key string, value interface{}) Logger {
	return DefaultLogger.WithField(key, value)
}

// WithFields adds multiple fields to the logger.
func WithFields(fields map[string]interface{}) Logger {
	return DefaultLogger.WithFields(fields)
}

// WithError adds an error to the logger.
func WithError(err error) Logger {
	return DefaultLogger.WithError(err)
}

// SetLevel sets the logging level.
func SetLevel(level string) {
	if l, ok := DefaultLogger.(*LogrusLogger); ok {
		l.SetLevel(level)
	}
}