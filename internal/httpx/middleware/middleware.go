// Package middleware holds the cross-cutting HTTP layers. The order they are
// mounted in is fixed in httpx.Router, not decided here.
package middleware

import "net/http"

// Middleware wraps a handler with one cross-cutting concern.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware left to right: the first entry is the outermost.
func Chain(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

// statusWriter records the status code for the logging layer.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}
