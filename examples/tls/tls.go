package tls

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

const (
	cert = "examples/tls/cert.pem"
	key  = "examples/tls/key.pem"
)

type Req struct {
	data []byte
	done chan bool
}

type MyListener chan MyConn

var ln = MyListener(make(chan MyConn))

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

func (c MyConn) Read(b []byte) (n int, err error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n = copy(b, c.data)
	c.data = c.data[n:]
	return
}

func (c MyConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (c MyConn) Close() error {
	close(c.done)
	return nil
}

func (c MyConn) LocalAddr() net.Addr {
	return &net.TCPAddr{net.IP{127, 0, 0, 1}, 49706, ""}
}

func (c MyConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{net.IP{127, 0, 0, 1}, 49706, ""}
}

func (c MyConn) SetDeadline(t time.Time) error {
	return nil
}

func (c MyConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c MyConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func init() {
	go func() {
		c, err := ioutil.ReadFile(cert)
		if err != nil {
			panic(err)
		}
		k, err := ioutil.ReadFile(key)
		if err != nil {
			panic(err)
		}
		cert, err := tls.X509KeyPair(c, k)
		if err != nil {
			panic(err)
		}
		tlsConfig := &tls.Config{
			NextProtos:   []string{"http/1.1"},
			Certificates: []tls.Certificate{cert},
		}
		tlsListener := tls.NewListener(ln, tlsConfig)
		http.HandleFunc("/", handler)
		if err := http.Serve(tlsListener, nil); err != nil {
			panic(err)
		}
	}()
}

var reply = []byte("hello")

func handler(w http.ResponseWriter, req *http.Request) {
	w.Write(reply)
}

func Fuzz(data []byte) int {
	done := make(chan bool)
	ln <- MyConn{data, done}
	<-done
	return 0
}
