package websocketclient

import (
	"golang.org/x/net/websocket"
	"io"
)

func Fuzz(data []byte) int {
	cfg, err := websocket.NewConfig("http://golang.org/", "http://golang.org/")
	if err != nil {
		panic(err)
	}
	//	Header:   http.Header{},
	conn := &Conn{data, false}
	ws, err := websocket.NewClient(cfg, conn)
	if err != nil {
		return 0
	}
	ws.Write([]byte("abc"))
	var buf [16]byte
	ws.Read(buf[:])
	ws.Read(buf[:])
	ws.Close()
	if !conn.closed {
		panic("conn is not closed")
	}
	return 1
}

type Conn struct {
	data   []byte
	closed bool
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n = copy(b, c.data)
	c.data = c.data[n:]
	return
}

func (c *Conn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (c *Conn) Close() error {
	if c.closed {
		panic("already closed")
	}
	c.closed = true
	return nil
}
