package common

import (
	"fmt"
	"github.com/efigence/rodrev/config"
	"github.com/urfave/cli"
	"net/url"
)

// MergeCliConfig merges(overrides mostly) cli and file config values
func MergeCliConfig(cfg *config.Config, c *cli.Context) {
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
	ca := u.Query().Get("ca")
	crt := u.Query().Get("cert")
	if len(ca) == 0 {
		ca = url.QueryEscape(cfg.CA)
	}
	if len(crt) == 0 {
		crt = url.QueryEscape(cfg.ClientCert)
	}
	if len(ca) > 0 {
		u.RawQuery = u.RawQuery + "&ca=" + ca
	}
	if len(crt) > 0 {
		u.RawQuery = u.RawQuery + "&cert=" + crt
	}
	cfg.MQAddress = u.String()

}
