package tlsclient

import (
	"crypto/tls"
	"io"
	"math/rand"
	"net"
	"time"
)

type MyConn struct {
	data []byte
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

type MathRandReader int

func (MathRandReader) Read(buf []byte) (int, error) {
	for i := range buf {
		buf[i] = byte(rand.Intn(256))
	}
	return len(buf), nil
}

func Fuzz(data []byte) int {
	rand.Seed(0)
	c := &MyConn{data}
	tc := tls.Client(c, &tls.Config{
		InsecureSkipVerify: true,
		Rand:               MathRandReader(0),
		Time:               func() time.Time { return time.Date(2000, 1, 1, 1, 1, 1, 1, nil) },
	})
	tc.Handshake()
	return 0
}
