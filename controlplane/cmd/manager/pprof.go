package main

import (
	"net/http"
	nethttppprof "net/http/pprof"
	"time"
)

const (
	defaultPprofReadHeaderTimeout = 5 * time.Second
	defaultPprofReadTimeout       = 30 * time.Second
	defaultPprofWriteTimeout      = 2 * time.Minute
	defaultPprofIdleTimeout       = 2 * time.Minute
	defaultPprofMaxHeaderBytes    = 32 << 10
)

func newPprofServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           newPprofHandler(),
		ReadHeaderTimeout: defaultPprofReadHeaderTimeout,
		ReadTimeout:       defaultPprofReadTimeout,
		WriteTimeout:      defaultPprofWriteTimeout,
		IdleTimeout:       defaultPprofIdleTimeout,
		MaxHeaderBytes:    defaultPprofMaxHeaderBytes,
	}
}

func newPprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", nethttppprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", nethttppprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", nethttppprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", nethttppprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", nethttppprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", nethttppprof.Trace)
	mux.Handle("GET /debug/pprof/{profile}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nethttppprof.Handler(r.PathValue("profile")).ServeHTTP(w, r)
	}))
	return mux
}
