package logger

import "go.uber.org/zap"

var (
	Logger *zap.Logger
	SugaredLogger *zap.SugaredLogger
)

func init() {
	Logger, _ := zap.NewProduction()
	SugaredLogger = Logger.Sugar()
}
