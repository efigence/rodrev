package common

import (
	"crypto/rand"
	"fmt"
	"github.com/efigence/rodrev/config"
	"github.com/spf13/cobra"
	"net/url"
	"strconv"
	"time"
)

func StringOrPanic(s string, err error) string {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return s
}

func BoolOrPanic(b bool, err error) bool {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return b
}
func DurationOrPanic(d time.Duration, err error) time.Duration {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return d
}

// RandomToken returns a short random string usable in a topic or a client id
func RandomToken(bytes int) string {
	blob := make([]byte, bytes)
	if _, err := rand.Read(blob); err != nil {
		// only used to keep names apart, so a timestamp is good enough
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return MapBytesToTopicTitle(blob)
}

// RedactURL returns URL with password replaced by a placeholder, safe to log.
// Unparseable URLs are redacted as a whole so a malformed one can't leak the password
func RedactURL(s string) string {
	if len(s) == 0 {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return "[unparseable url]"
	}
	return u.Redacted()
}

// MergeCliConfig merges(overrides mostly) cli and file config values
func MergeCliConfig(cfg *config.Config, cmd *cobra.Command) {
	c := cmd.Flags()

	if len(StringOrPanic(c.GetString("mqtt-url"))) > 0 {
		cfg.MQAddress = StringOrPanic(c.GetString("mqtt-url"))
	}

	if len(cfg.MQAddress) == 0 {
		cfg.MQAddress = "tcp://mqtt:mqtt@127.0.0.1:1883"
	}
	u, err := url.Parse(cfg.MQAddress)
	if err != nil {
		// the parse error would carry the url, and with it the password
		panic(fmt.Sprintf("can't parse MQ url [%s]: %s", RedactURL(cfg.MQAddress), causeOf(err)))
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
