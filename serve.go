package main

import (
	"context"
	"fmt"
	"net/http"
	"runtime"

	"github.com/shurcooL/githubv4"
	"k8s.io/klog/v2"
)

type ServerConfig struct {
	Addr     string
	V4Client *githubv4.Client
}

func serve(ctx context.Context, sc *ServerConfig) {
	s := &Server{V4Client: sc.V4Client}
	http.HandleFunc("/", s.Root())
	http.HandleFunc("/healthz", s.Healthz())
	http.HandleFunc("/threadz", s.Threadz())
	klog.Infof("Listening on %s ...", sc.Addr)
	http.ListenAndServe(sc.Addr, nil)
}

type Server struct {
	V4Client *githubv4.Client
}

func (s *Server) Root() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		klog.Infof("request: %+v", r)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "w00t")
	}
}

func (s *Server) Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) Threadz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		klog.Infof("GET %s: %v", r.URL.Path, r.Header)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(stack()); err != nil {
			klog.Errorf("writing threadz response: %d", err)
		}
	}
}
func stack() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}
