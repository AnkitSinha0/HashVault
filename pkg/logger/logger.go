package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)
// global log var
var Log *zap.Logger

func Init(env string) {
	var cfg zap.Config

	if env == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	var err error
	Log, err = cfg.Build()
	if err != nil {
		panic("failed to initialise logger: " + err.Error())
	}
}

func Sync() {
	_ = Log.Sync()
}
