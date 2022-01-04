package main

import (
	"fmt"
	"github.com/efigence/rodrev/util"
	"github.com/spf13/cobra"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/XANi/go-yamlcfg"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	uuid "github.com/satori/go.uuid"
	"github.com/zerosvc/go-zerosvc"
)

func stringOrPanic(s string, err error) string {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return s
}

func boolOrPanic(b bool, err error) bool {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return b
}
func durationOrPanic(d time.Duration, err error) time.Duration {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return d
}
func Init(cmd *cobra.Command) (config.Config, common.Runtime) {
	c := cmd.Flags()
	cfgFiles := []string{
		"$HOME/.config/rodrev/client.conf",
		"/etc/rodrev/client.conf",
		"./cfg/client-local.yaml",
		"./cfg/client.yaml",
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
	debug = boolOrPanic(c.GetBool("debug"))
	quiet = boolOrPanic(c.GetBool("quiet"))
	InitLog()

	log.Infof("config: %s", cfg.GetConfigPath())

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

	runtime := common.Runtime{
		Node: node,
		// TODO load from cert if possible
		FQDN:     util.GetFQDN(),
		MQPrefix: cfg.MQPrefix,
		Log:      log,
		Debug:    debug,
		Cfg:      cfg,
	}
	outputMode := stringOrPanic(c.GetString("output-format"))
	outputModeRe := regexp.MustCompile(
		"^" +
			strings.Join([]string{outCsv, outJson, outStderr}, "|") +
			"$")
	if !outputModeRe.MatchString(outputMode) {
		log.Panicf("output-format [%s] must match %s", outputMode, outputModeRe)
	}
	return cfg, runtime

}
