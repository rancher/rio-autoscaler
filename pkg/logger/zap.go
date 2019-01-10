package logger

import (
	"go.uber.org/zap"
)

var (
	SugaredLogger *zap.SugaredLogger
)

func InitLogger(debug bool) error {
	var Logger *zap.Logger
	var err error
	if debug {
		Logger, err = zap.NewDevelopment()
	} else {
		Logger, err = zap.NewProduction()
	}
	if err != nil {
		panic(err)
	}
	SugaredLogger = Logger.Sugar()
	return nil
}
