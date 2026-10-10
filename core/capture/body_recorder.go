package capture

import (
	"errors"
	"io"
	"sync"
)

// bodyRecorder relays a body unchanged while keeping a copy of its first
// limit bytes. Unlike reading the body up front, it never delays a streaming
// response (server-sent events, chunked downloads) on the capture limit.
type bodyRecorder struct {
	source io.ReadCloser
	limit  int

	mu        sync.Mutex
	buf       []byte
	total     int64
	truncated bool
	readErr   error

	once   sync.Once
	onDone func(*bodyRecorder)
}

func newBodyRecorder(source io.ReadCloser, limit int, onDone func(*bodyRecorder)) *bodyRecorder {
	return &bodyRecorder{source: source, limit: limit, onDone: onDone}
}

func (r *bodyRecorder) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if n > 0 {
		r.mu.Lock()
		r.total += int64(n)
		if room := r.limit - len(r.buf); room > 0 {
			chunk := p[:n]
			if len(chunk) > room {
				chunk = chunk[:room]
				r.truncated = true
			}
			r.buf = append(r.buf, chunk...)
		} else {
			r.truncated = true
		}
		r.mu.Unlock()
	}
	if err != nil {
		if !errors.Is(err, io.EOF) {
			r.mu.Lock()
			r.readErr = err
			r.mu.Unlock()
		}
		r.finish()
	}
	return n, err
}

func (r *bodyRecorder) Close() error {
	err := r.source.Close()
	r.finish()
	return err
}

func (r *bodyRecorder) finish() {
	r.once.Do(func() {
		if r.onDone != nil {
			r.onDone(r)
		}
	})
}

// snapshot returns a copy of what has been captured so far, the number of
// bytes relayed, whether the copy was cut at the limit, and any read error.
func (r *bodyRecorder) snapshot() ([]byte, int64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.buf...), r.total, r.truncated, r.readErr
}
