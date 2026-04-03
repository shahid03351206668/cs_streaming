package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

func InitLogger() {
	if err := os.MkdirAll("./logs", 0755); err != nil {
		panic("failed to create logs directory: " + err.Error())
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// General log: INFO+ — all application events (app.log)
	appWriter := &lumberjack.Logger{
		Filename:   "./logs/app.log",
		MaxSize:    20,
		MaxBackups: 7,
		MaxAge:     30,
		Compress:   true,
	}

	// Error log: WARN+ — warnings and errors only (error.log)
	errorWriter := &lumberjack.Logger{
		Filename:   "./logs/error.log",
		MaxSize:    10,
		MaxBackups: 14,
		MaxAge:     60,
		Compress:   true,
	}

	appCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(appWriter),
		zap.InfoLevel,
	)

	errorCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(errorWriter),
		zap.WarnLevel,
	)

	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)

	core := zapcore.NewTee(appCore, errorCore, consoleCore)
	Log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

// Sync flushes any buffered log entries. Call this on server shutdown.
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
