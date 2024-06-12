package util

import (
	"fmt"
	"github.com/k0kubun/pp/v3"
	"github.com/zerosvc/go-zerosvc"
)

func PPEvent(ev *zerosvc.Event) string {
	return fmt.Sprintf("r: %s\nheaders: %s\nrt: %s\nbody: %s\n",
		ev.RoutingKey,
		pp.Sprint(ev.Headers),
		ev.ReplyTo,
		string(ev.Body))
}
