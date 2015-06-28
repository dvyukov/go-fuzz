package httpserver

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"
)

type MyListener chan *MyConn

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
	data    []byte
	done    chan bool
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

type LogPipe struct {
	c chan []byte
}

func (p *LogPipe) Write(data []byte) (int, error) {
	p.c <- append([]byte{}, data...)
	return len(data), nil
}

func (p *LogPipe) ReadAll() []byte {
	var data []byte
	for {
		select {
		case d := <-p.c:
			data = append(data, d...)
		default:
			return data
		}
	}
}

func init() {
	http.HandleFunc("/", handler)
	s := &http.Server{
		ErrorLog: log.New(logpipe, "", 0),
	}
	go func() {
		if err := s.Serve(ln); err != nil {
			panic(err)
		}
	}()
}

func handler(w http.ResponseWriter, req *http.Request) {
	w.Write(reply)
}

var (
	logpipe = &LogPipe{c: make(chan []byte, 1000)}
	ln      = MyListener(make(chan *MyConn))
	reply   = []byte("hello")
)

func Fuzz(data []byte) int {
	c := &MyConn{data: data, done: make(chan bool)}
	ln <- c
	<-c.done
	http.DefaultTransport.(*http.Transport).CloseIdleConnections()
	runtime.Gosched()
	if data := logpipe.ReadAll(); bytes.Contains(data, []byte("http: panic serving")) {
		fmt.Printf("%s\n", data)
		panic("serving panicked")
	}
	if runtime.NumGoroutine() > 50 {
		panic("leaking goroutines")
	}
	if c.written {
		return 1
	}
	return 0
}
