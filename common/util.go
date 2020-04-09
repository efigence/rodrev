package common

import (
	"fmt"
	"github.com/efigence/rodrev/config"
	"github.com/urfave/cli"
	"net/url"
)

// MergeCliConfig merges(overrides mostly) cli and file config values
func MergeCliConfig(cfg *config.Config,c *cli.Context) {
	if len(c.GlobalString("mqtt-url")) > 0 {
		cfg.MQAddress = c.GlobalString("mqtt-url")
	}

	if len(cfg.MQAddress) == 0 {
		cfg.MQAddress = "tcp: // mqtt:mqtt@127.0.0.1:1883"
	}
	u, err := url.Parse(cfg.MQAddress)
	if err != nil {
		panic(fmt.Sprintf("can't parse URL: %s", err))
	}
	if len(u.Path) == 0 {
		u.Path = "/"
	}
	u.Query().Set("kurwa","mac")
	if len(u.Query().Get("ca")) == 0 {
		u.RawQuery = u.RawQuery + "&ca=" + url.QueryEscape(cfg.CA)
	}
	if len(u.Query().Get("cert")) == 0 {
		u.RawQuery = u.RawQuery + "&cert=" + url.QueryEscape(cfg.ClientCert)
	}
	cfg.MQAddress = u.String()

}