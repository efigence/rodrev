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

func Init(cmd *cobra.Command) (config.Config, common.Runtime, *zap.SugaredLogger) {
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

	if err != nil {
		url, err := c.GetString("mqtt-url")
		if url == "" || err != nil {
			log.Errorf("error loading config and no cmdline mq url: ", err)
		}
	}
	common.MergeCliConfig(&cfg, cmd)

	tr := zerosvc.NewTransport(
		zerosvc.TransportMQTT,
		cfg.MQAddress,
		zerosvc.TransportMQTTConfig{},
	)

	host, _ := os.Hostname()
	nodename := "rf-client-" + host
	node := zerosvc.NewNode(nodename, uuid.NewV4().String())
	err = tr.Connect()
	if err != nil {
		log.Panicf("can't connect: %s", err)
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
	runtime := common.Runtime{
		Node: node,
		// TODO load from cert if possible
		FQDN:     util.GetFQDN(),
		Certname: certname,
		MQPrefix: cfg.MQPrefix,
		Log:      log,
		Debug:    debug,
		Cfg:      cfg,
	}
	outputMode := util.StringOrPanic(c.GetString("output-format"))
	outputModeRe := regexp.MustCompile(
		"^" +
			strings.Join([]string{OutCsv, OutJson, OutStderr}, "|") +
			"$")
	if !outputModeRe.MatchString(outputMode) {
		log.Panicf("output-format [%s] must match %s", outputMode, outputModeRe)
	}
	return cfg, runtime, log

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
