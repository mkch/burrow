package statushook

import (
	"log"
	"net/http"
)

// Objects implementing the Hook interface can be used by Handler function to
// hook http response.
type Hook interface {
	// Hook is called before a status code is written to w, tipically a call
	// to w.WriterHeader().
	// w can be used to write a different response to the client.
	// The original response will be completly discarded if w.Write() or
	// w.WriteHeader() is called in this function. The original response will be
	// written with the modified header if w.Header() is modified witout calling
	// w.Write() or w.WriteHeader().
	Hook(code int, w http.ResponseWriter, r *http.Request)
}

// The HookFunc type is an adapter to allow the use of ordinary functions as
// Hook interface. If f is a function with the appropriate signature,
// HookFunc(f) is a Hook object that calls f.
type HookFunc func(code int, w http.ResponseWriter, r *http.Request)

// Hook calls f(code, w, r).
func (f HookFunc) Hook(code int, w http.ResponseWriter, r *http.Request) {
	f(code, w, r)
}

type responseWriter struct {
	// The original ResponseWriter
	http.ResponseWriter
	// The request that this Writer is handling.
	r *http.Request
	// The hook
	hook Hook
	// Hooked or not.
	hooked bool
	// WriteHeader has been called or not.
	wroteHeader bool
	// Invoking hook.
	inHook bool
}

func (w *responseWriter) Write(data []byte) (int, error) {
	// Called in hook.
	if w.inHook {
		// No further response after hook.
		w.hooked = true
		return w.ResponseWriter.Write(data)
	}
	if w.hooked {
		return len(data), nil // Black hole.
	}
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		log.Print("http: multiple response.WriteHeader calls")
		return
	}
	// Called in hook.
	if w.inHook {
		w.wroteHeader = true
		// No further response after hook.
		w.hooked = true
		w.ResponseWriter.WriteHeader(code)
	} else { // Called out of hook
		if w.hooked {
			return // Black hole.
		}
		// Invok the hook.
		w.inHook = true
		w.hook.Hook(code, w, w.r)
		w.inHook = false
		// No further process if hooked.
		if !w.hooked {
			w.wroteHeader = true
			w.ResponseWriter.WriteHeader(code)
		}
	}
}

// Handler function returns a wrapped http.Handler which calls hook.Hook()
// with the status code before the status code is written to response, giving
// hook.Hook() a chance to midify the response header, or replace the entire
// response.
// See the Hook interface for details.
func Handler(handler http.Handler, hook Hook) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookedWriter := &responseWriter{ResponseWriter: w, r: r, hook: hook}
		handler.ServeHTTP(hookedWriter, r)
	})
}
