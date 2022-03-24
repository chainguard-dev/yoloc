package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"html/template"
	"net/http"
	"runtime"

	"github.com/buildkite/terminal-to-html/v3"
	"github.com/shurcooL/githubv4"
	"k8s.io/klog/v2"
)

//go:embed yolo.tmpl
var yoloTmpl string

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
		klog.Infof("%s: %s %s", r.RemoteAddr, r.Method, r.URL)

		tmpl, err := template.New("yolo").Parse(yoloTmpl)
		if err != nil {
			klog.Errorf("tmpl error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		repo := "chainguard-dev/yolo"
		image := "tstromberg/yoloc"
		work := false

		if len(r.URL.Query()["repo"]) > 0 {
			repo = r.URL.Query()["repo"][0]
			work = true
		}
		if len(r.URL.Query()["image"]) > 0 {
			image = r.URL.Query()["image"][0]
			work = true
		}

		var bs bytes.Buffer
		bw := bufio.NewWriter(&bs)
		// hohoho!
		showBanner(bw)

		if work {
			klog.Infof("Running checks for %s / %s", repo, image)
			_, err = runChecks(r.Context(), bw, &Config{
				Github:   repo,
				Image:    image,
				V4Client: s.V4Client,
			})
		} else {
			bw.Write([]byte("\nWaiting for submission ...\n"))
		}

		bw.Flush()
		output := terminal.Render(bs.Bytes())
		klog.Infof("output:\n%s", output)

		data := struct {
			Title string
			Out   template.HTML
			Repo  string
			Image string
		}{
			Title: "YOLO compliance checker",
			Repo:  repo,
			Image: image,
			Out:   template.HTML(output),
		}

		var tpl bytes.Buffer
		if err = tmpl.Execute(&tpl, data); err != nil {
			klog.Errorf("tmpl error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		out := tpl.Bytes()
		w.WriteHeader(http.StatusOK)
		w.Write(out)
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
