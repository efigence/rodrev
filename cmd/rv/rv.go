package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var version string
var log *zap.SugaredLogger
var debug = true
var quiet = false
var exit = make(chan bool, 1)

const (
	outStderr = "stderr"
	outCsv    = "csv"
	outJson   = "json"
)

func init() {
	InitLog()
}

func InitLog() {
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	// naive systemd detection. Drop timestamp if running under it
	// if os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != "" {
	// 	consoleEncoderConfig.TimeKey = ""
	// }
	consoleEncoderConfig.TimeKey = ""
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
	consoleStderr := zapcore.Lock(os.Stderr)
	_ = consoleStderr
	filterAll := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool { return true })
	filterHighPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})
	filterQuiet := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.WarnLevel
	})
	filterInfo := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.InfoLevel
	})
	var logger *zap.Logger
	if debug {
		core := zapcore.NewCore(consoleEncoder, os.Stderr, filterAll)
		logger = zap.New(core).WithOptions(
			zap.Development(),
			zap.AddCaller(),
			zap.AddStacktrace(filterHighPriority),
		)
	} else if quiet {
		core := zapcore.NewCore(consoleEncoder, os.Stderr, filterQuiet)
		logger = zap.New(core).WithOptions(
			zap.AddCaller(),
		)
	} else {
		core := zapcore.NewCore(consoleEncoder, os.Stderr, filterInfo)
		logger = zap.New(core).WithOptions(
			zap.AddCaller(),
		)
	}
	log = logger.Sugar()
}

func main() {
	cobraInit()
	err :=rootCmd.Execute()
	if err != nil {
		log.Errorf("error parsing commands: %s", err)
		os.Exit(1)
	}

}
