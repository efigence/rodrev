package main

import (
	"github.com/XANi/go-yamlcfg"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/util"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/cobra"
	"github.com/zerosvc/go-zerosvc"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

func Init(cmd *cobra.Command) (config.Config, common.Runtime) {
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
	debug = util.BoolOrPanic(c.GetBool("debug"))
	quiet = util.BoolOrPanic(c.GetBool("quiet"))
	InitLog()

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
			strings.Join([]string{outCsv, outJson, outStderr}, "|") +
			"$")
	if !outputModeRe.MatchString(outputMode) {
		log.Panicf("output-format [%s] must match %s", outputMode, outputModeRe)
	}

	return cfg, runtime

}
