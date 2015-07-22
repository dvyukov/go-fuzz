package websocketserver

import (
	"golang.org/x/net/websocket"
	"io"
	"net"
	"net/http"
	"time"
)

func Fuzz(data []byte) int {
	conn := &MyConn{data, make(chan bool)}
	ln <- conn
	<-conn.done
	return 0
}

func init() {
	http.Handle("/", websocket.Handler(func(ws *websocket.Conn) {
		io.Copy(ws, ws)
	}))
	go func() {
		http.Serve(ln, nil)
		panic("serve returned")
	}()
}

type MyListener chan *MyConn

var ln = MyListener(make(chan *MyConn))

func (ln MyListener) Accept() (c net.Conn, err error) {
	return <-ln, nil
}

func (ln MyListener) Close() error {
	return nil
}

func (ln MyListener) Addr() net.Addr {
	return &net.TCPAddr{net.IP{127, 0, 0, 1}, 49706, ""}
}

type MyConn struct {
	data []byte
	done chan bool
}

func (c *MyConn) Read(b []byte) (n int, err error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n = copy(b, c.data)
	c.data = c.data[n:]
	return
}

func (c *MyConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (c *MyConn) Close() error {
	close(c.done)
	return nil
}

func (c *MyConn) LocalAddr() net.Addr {
	return &net.TCPAddr{net.IP{127, 0, 0, 1}, 49706, ""}
}

func (c *MyConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{net.IP{127, 0, 0, 1}, 49706, ""}
}

func (c *MyConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *MyConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *MyConn) SetWriteDeadline(t time.Time) error {
	return nil
}
