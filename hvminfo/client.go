package hvminfo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/pkg/term"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"time"
)

type ConfigClient struct {
	Port           string             `yaml:"port"`
	Speed          int                `yaml:"baudrate"`
	PuppetFactPath string             `yaml:"puppet_fact_path"`
	Logger         *zap.SugaredLogger `yaml:"-"`
	Version        string             `yaml:"-"`
}

func RunClient(cfg *ConfigClient) error {
	if cfg == nil {
		return fmt.Errorf("no config passed")
	}
	if len(cfg.Port) < 1 {
		return fmt.Errorf("need Port parameter")
	}
	t, err := term.Open(cfg.Port, term.Speed(cfg.Speed), term.RawMode, term.FlowControl(term.NONE))
	if err != nil {
		return fmt.Errorf("error opening %s: %s", cfg.Port, err)
	}
	go func() {
		scanner := bufio.NewScanner(t)
		go func() {
			for {
				_, err := t.Write([]byte(CmdInfo))
				if err != nil {
					cfg.Logger.Errorf("error writing command to %s", cfg.Port)
				}
				time.Sleep(time.Minute * 5)
			}
		}()
		for scanner.Scan() {
			line := scanner.Bytes()
			var i HVMInfo
			err := json.Unmarshal(line, &i)
			if err != nil {
				cfg.Logger.Infof("error unmarshalling [%s] \n", line)
				continue
			}
			if len(cfg.PuppetFactPath) > 0 {
				if len(i.FQDN) > 0 {
					var f Facts
					f.VmHost = i.FQDN
					f.RodrevVersion = cfg.Version
					data, err := yaml.Marshal(f)
					if err != nil {
						cfg.Logger.Errorf("error marshalling data: %s", err)
					}
					err = ioutil.WriteFile(cfg.PuppetFactPath+".tmp", data, 0644)
					if err != nil {
						cfg.Logger.Errorf("error writing tmpfile: %s", err)
					}
					err = os.Rename(cfg.PuppetFactPath+".tmp", cfg.PuppetFactPath)
					if err != nil {
						cfg.Logger.Errorf("error renaming tmpfile: %s", err)
					}
				}
			}
		}
		cfg.Logger.Infof("serial reader exited\n")
	}()
	return nil
}
