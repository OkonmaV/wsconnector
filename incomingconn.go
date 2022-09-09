package wsconnector

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/big-larry/suckutils"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/mailru/easygo/netpoll"
)

type EpollWSConnector struct {
	conn    net.Conn
	desc    *netpoll.Desc
	handler WsHandler
	sync.RWMutex
	isclosed bool
}

var DefaultUpgrader ws.Upgrader

func createUpgrader(v UpgradeReqChecker) ws.Upgrader {
	return ws.Upgrader{
		OnRequest: func(uri []byte) error {
			if sc := v.CheckPath(uri); sc != 200 {
				return ws.RejectConnectionError(ws.RejectionStatus(int(sc)))
			}
			return nil
		},
		OnHost: func(host []byte) error {
			if sc := v.CheckHost(host); sc != 200 {
				return ws.RejectConnectionError(ws.RejectionStatus(int(sc)))
			}
			return nil
		},
		OnHeader: func(key, value []byte) error {
			if sc := v.CheckHeader(key, value); sc != 200 {
				return ws.RejectConnectionError(ws.RejectionStatus(int(sc)))
			}
			return nil
		},
		OnBeforeUpgrade: func() (header ws.HandshakeHeader, err error) {
			if sc := v.CheckBeforeUpgrade(); sc != 200 {
				return nil, ws.RejectConnectionError(ws.RejectionStatus(int(sc)))
			}
			return nil, nil
		},
	}
}

// upgrades connection and adds it to epoll
func NewWSConnectorWithUpgrade(conn net.Conn, handler WsHandler) (*EpollWSConnector, error) {
	if conn == nil {
		return nil, ErrNilConn
	}
	// upgrade сам отправляет респонс
	if upgrReqChecker, ok := handler.(UpgradeReqChecker); ok {
		if _, err := createUpgrader(upgrReqChecker).Upgrade(conn); err != nil {
			return nil, err
		}
	} else {
		if _, err := ws.DefaultUpgrader.Upgrade(conn); err != nil {
			return nil, err
		}
	}

	desc, err := netpoll.HandleRead(conn)
	if err != nil {
		return nil, err
	}

	connector := &EpollWSConnector{conn: conn, desc: desc, handler: handler}

	return connector, nil
}

func Dial(ctx context.Context, urlstr string, handler WsHandler) (*EpollWSConnector, error) {
	conn, _, _, err := ws.Dialer{Timeout: time.Second * 5}.Dial(ctx, urlstr)
	if err != nil {
		return nil, err
	}

	desc, err := netpoll.HandleRead(conn)
	if err != nil {
		return nil, err
	}

	connector := &EpollWSConnector{conn: conn, desc: desc, handler: handler}

	return connector, nil
}

func (connector *EpollWSConnector) StartServing() error {
	return poller.Start(connector.desc, connector.handle)
}

// MUST be called after StartServing() failure to prevent memory leak!
func (connector *EpollWSConnector) ClearFromCache() {
	connector.Lock()
	defer connector.Unlock()

	connector.stopserving()
}

func (connector *EpollWSConnector) handle(e netpoll.Event) {
	defer poller.Resume(connector.desc)

	if e&(netpoll.EventReadHup|netpoll.EventHup) != 0 {
		connector.Close(errors.New(e.String()))
		return
	}

	connector.Lock() //

	if connector.isclosed {
		connector.Unlock() //
		return
	}

	connector.conn.SetReadDeadline(time.Now().Add(time.Second * 5)) // TODO: for test???
	h, r, err := wsutil.NextReader(connector.conn, thisSide)
	if err != nil {
		connector.Unlock() //
		connector.Close(err)
		return
	}
	if h.OpCode.IsControl() {
		if err := wsutil.ControlFrameHandler(connector.conn, thisSide)(h, r); err != nil {
			connector.Unlock() //
			connector.Close(err)
		}
		return
	}
	//time.Sleep(time.Second)
	message := connector.handler.NewMessage()
	if err := message.ReadWS(r, h); err != nil {
		connector.Unlock() //
		connector.Close(errors.New(suckutils.ConcatTwo("message.Read() err: ", err.Error())))
		return
	}
	// payload, _, err := wsutil.ReadData(connector.conn, thisSide)
	// if err != nil {
	// 	connector.Unlock() //
	// 	connector.Close(err)
	// 	return
	// }
	connector.Unlock() //

	// if len(payload) == 0 {
	// 	connector.Close(ErrEmptyPayload)
	// 	return
	// }

	if pool != nil {
		pool.Schedule(func() {
			if err := connector.handler.Handle(message); err != nil {
				connector.Close(err)
			}
		})
		return
	}
	if err = connector.handler.Handle(message); err != nil {
		connector.Close(err)
	}
}

func (connector *EpollWSConnector) Send(message []byte) error {
	if connector.IsClosed() {
		return ErrClosedConnector
	}
	//connector.conn.SetWriteDeadline(time.Now().Add(time.Second))
	connector.Lock()
	defer connector.Unlock()
	return wsutil.WriteMessage(connector.conn, thisSide, ws.OpBinary, message)
}

func (connector *EpollWSConnector) Close(reason error) { // TODO: можно добавить отправку OpClose перед закрытием соединения
	connector.Lock()
	defer connector.Unlock()

	if connector.isclosed {
		return
	}
	connector.stopserving()
	connector.handler.HandleClose(reason)
}

func (connector *EpollWSConnector) stopserving() error {
	connector.isclosed = true
	poller.Stop(connector.desc)
	connector.desc.Close()
	return connector.conn.Close()
}

// call in HandleClose() will cause deadlock
func (connector *EpollWSConnector) IsClosed() bool {
	connector.RLock()
	defer connector.RUnlock()
	return connector.isclosed
}

func (connector *EpollWSConnector) RemoteAddr() net.Addr {
	return connector.conn.RemoteAddr()
}
