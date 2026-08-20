package clinit

import (
	"github.com/XANi/go-yamlcfg"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/util"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/cobra"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

const (
	OutStderr = "stderr"
	OutCsv    = "csv"
	OutJson   = "json"
)

// Init loads config, connects to the MQ and returns a runtime ready for use.
// It panics when there is no usable config or the broker can not be reached
func Init(cmd *cobra.Command) (config.Config, common.Runtime, *zap.SugaredLogger) {
	cfg, log := initConfig(cmd, false)
	runtime := connect(cmd, cfg, log)
	return cfg, runtime, log
}

// InitOffline loads config and logger the same way Init does and validates
// output format, but never connects to the MQ and does not panic when there is
// no config file. For commands that can do useful work without a cluster
func InitOffline(cmd *cobra.Command) (config.Config, *zap.SugaredLogger) {
	return initConfig(cmd, true)
}

func initConfig(cmd *cobra.Command, offline bool) (config.Config, *zap.SugaredLogger) {
	c := cmd.Flags()
	cfgFiles := []string{
		"$HOME/.config/rodrev/client.conf",
		"/etc/rodrev/client.conf",
		"./cfg/client-local.yaml",
		"./cfg/client.yaml",
	}
	if len(os.Getenv("RV_CONFIG")) > 0 {
		cfgFiles = append([]string{os.Getenv("RV_CONFIG")}, cfgFiles...)
	}
	debug := util.BoolOrPanic(c.GetBool("debug"))
	quiet := util.BoolOrPanic(c.GetBool("quiet"))
	log := InitLog(debug, quiet)
	userCfg, err := c.GetString("config")
	if err == nil && len(userCfg) > 0 {
		if _, err := os.Stat(userCfg); os.IsNotExist(err) {
			log.Panicf("config file %s does not exist", userCfg)
		}
		cfgFiles = append([]string{userCfg}, cfgFiles...)
	}
	var cfg config.Config
	cfg.Logger = log
	err = yamlcfg.LoadConfig(cfgFiles, &cfg)

	if err != nil && !offline {
		url, err := c.GetString("mqtt-url")
		if url == "" || err != nil {
			log.Errorf("error loading config and no cmdline mq url: ", err)
		}
	}
	common.MergeCliConfig(&cfg, cmd)
	outputMode := util.StringOrPanic(c.GetString("output-format"))
	outputModeRe := regexp.MustCompile(
		"^" +
			strings.Join([]string{OutCsv, OutJson, OutStderr}, "|") +
			"$")
	if !outputModeRe.MatchString(outputMode) {
		log.Panicf("output-format [%s] must match %s", outputMode, outputModeRe)
	}
	return cfg, log
}

func connect(cmd *cobra.Command, cfg config.Config, log *zap.SugaredLogger) common.Runtime {
	c := cmd.Flags()
	debug := util.BoolOrPanic(c.GetBool("debug"))
	tr := zerosvc.NewTransport(
		zerosvc.TransportMQTT,
		cfg.MQAddress,
		zerosvc.TransportMQTTConfig{},
	)

	host, _ := os.Hostname()
	nodename := "rf-client-" + host
	node := zerosvc.NewNode(nodename, uuid.NewV4().String())
	log.Debugf("connecting to queue at %s", common.RedactURL(cfg.MQAddress))
	err := tr.Connect()
	if err != nil {
		log.Panicf("can't connect to queue at %s: %s", common.RedactURL(cfg.MQAddress), err)
	}
	node.SetTransport(tr)
	certname := ""
	if len(cfg.ClientCert) > 0 {
		cert, err := ioutil.ReadFile(cfg.ClientCert)
		if err != nil {
			log.Panicf("could not load cert %s: %w", cfg.ClientCert, err)
		}
		certname = util.GetCNFromCert(cert)
	}
	if len(certname) == 0 {
		log.Infof("config: %s", cfg.GetConfigPath())
		certname = util.GetFQDN()
	} else {
		log.Infof("config: %s, cert: %s", cfg.GetConfigPath(), certname)
	}
	return common.Runtime{
		Node: node,
		// TODO load from cert if possible
		FQDN:     util.GetFQDN(),
		Certname: certname,
		MQPrefix: cfg.MQPrefix,
		Log:      log,
		Debug:    debug,
		Cfg:      cfg,
	}
}

func InitLog(debug, quiet bool) *zap.SugaredLogger {
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
	return logger.Sugar()
}
