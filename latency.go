package spdy

import (
	"io"
	"net/http"
	"sync"
	"time"
)

type writeFlusher interface {
	io.Writer
	http.Flusher
}

type maxLatencyWriter struct {
	dst     writeFlusher
	latency time.Duration

	lk   sync.Mutex // protects init of done, as well Write + Flush
	done chan bool
}

func MaxLatencyWriter(w io.Writer, latency time.Duration) io.Writer {
	if latency <= 0 {
		return w
	}

	dst, ok := w.(writeFlusher)
	if !ok {
		return w
	}

	return &maxLatencyWriter{
		dst:     dst,
		latency: latency,
	}
}

func (m *maxLatencyWriter) flushLoop() {
	t := time.NewTicker(m.latency)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			m.lk.Lock()
			m.dst.Flush()
			m.lk.Unlock()
		case <-m.done:
			return
		}
	}
	panic("unreached")
}

func (m *maxLatencyWriter) Write(p []byte) (n int, err error) {
	m.lk.Lock()
	defer m.lk.Unlock()
	if m.done == nil {
		m.done = make(chan bool)
		go m.flushLoop()
	}
	n, err = m.dst.Write(p)
	if err != nil {
		m.done <- true
	}
	return
}

type maxLatencyReader struct {
	w   maxLatencyWriter
	src io.Reader
}

func MaxLatencyReader(r io.Reader, latency time.Duration) io.Reader {
	if latency <= 0 {
		return r
	}

	l := &maxLatencyReader{}
	l.src = r
	l.w.latency = latency
	return l
}

func (m *maxLatencyReader) Read(b []byte) (n int, err error) {
	return m.src.Read(b)
}

func (m *maxLatencyReader) WriteTo(w io.Writer) (n int64, err error) {
	if dst, ok := w.(writeFlusher); ok {
		m.w.dst = dst
		return io.Copy(&m.w, m.src)
	}

	return io.Copy(w, m.src)
}
