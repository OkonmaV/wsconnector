package wsconnector

import (
	"io"

	"github.com/gobwas/ws"
)

type BasicMessage struct{ Payload []byte }

func NewBasicMessage() MessageReader {
	return &BasicMessage{}
}

func (m BasicMessage) ReadWS(r io.Reader, h ws.Header) error {
	m.Payload = make([]byte, h.Length)
	_, err := io.ReadFull(r, m.Payload)
	return err
}
