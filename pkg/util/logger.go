package util

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitZapLog() *zap.Logger {
	config := zap.NewProductionConfig()
	config.DisableStacktrace = true
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.DateTime + ".000")
	config.Encoding = "console"
	config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	logger, _ := config.Build()
	return logger
}
