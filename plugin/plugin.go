package plugin

import "github.com/zerosvc/go-zerosvc"

type Plugin interface {
	StartServer(evCh chan zerosvc.Event)
}
