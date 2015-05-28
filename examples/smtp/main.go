package smtp

import (
	"io"
	"net"
	"net/smtp"
	"time"
)

func Fuzz(data []byte) int {
	conn := &MyConn{data, false, false}
	defer func() {
		if !conn.closed {
			panic("connection is not closed")
		}
	}()
	c, err := smtp.NewClient(conn, "golang.org")
	if err != nil {
		return 0
	}
	defer c.Close()
	if err = c.Hello("localhost"); err != nil {
		return 0
	}
	if err = c.Auth(smtp.PlainAuth("identiry", "username", "password", "host")); err != nil {
		return 1
	}
	if err = c.Mail("gopher@golang.org"); err != nil {
		return 1
	}
	if err = c.Rcpt("gopher@golang.org"); err != nil {
		return 1
	}
	w, err := c.Data()
	if err != nil {
		return 1
	}
	_, err = w.Write([]byte("message"))
	if err != nil {
		return 1
	}
	if err = w.Close(); err != nil {
		return 1
	}
	if err = c.Quit(); err != nil {
		return 1
	}
	return 2
}

type MyConn struct {
	data    []byte
	closed  bool
	written bool
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
	c.written = true
	return len(b), nil
}

func (c *MyConn) Close() error {
	c.closed = true
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
