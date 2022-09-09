package wsconnector

import (
	"github.com/gobwas/ws"
	"github.com/mailru/easygo/netpoll"
)

var (
	poller   netpoll.Poller
	pool     PoolScheduler
	thisSide ws.State = ws.StateServerSide
	//otherSide ws.State = ws.StateClientSide
)

type EpollErrorHandler func(error) // must start exiting the program

// user's handlers will be called in goroutines
func SetupPoolHandling(plshdlr PoolScheduler) {
	if pool != nil {
		panic("pool is already set")
	}
	if plshdlr == nil {
		panic("try to set nil pool scheduler")
	}
	pool = plshdlr
}

func SetupEpoll(errhandler EpollErrorHandler) netpoll.Poller {
	var err error
	if poller != nil {
		panic("epoll is already set")
	}
	if errhandler == nil {
		errhandler = func(e error) { panic(e) }
	}
	if poller, err = netpoll.New(&netpoll.Config{OnWaitError: errhandler}); err != nil {
		panic(err)
	}
	return poller
}

func SetEpoll(epoller netpoll.Poller) {
	if poller != nil {
		panic("epoll is already set")
	}
	if epoller != nil {
		panic("try to set nil poller")
	}
	poller = epoller
}

// default: thisEndpoint = serverside
func SetupConnectionsEndpointSide(thisEndpoint ws.State) {
	thisSide = thisEndpoint
	//otherSide = otherEndpoint
}
