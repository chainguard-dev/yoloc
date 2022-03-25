package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

var (
	repoFlag   = flag.String("repo", "google/triage-party", "Github repo to check")
	imageFlag  = flag.String("image", "", "image to check")
	serveFlag  = flag.Bool("serve", false, "yoloc webserver mode")
	portFlag   = flag.Int("port", 8080, "serve yoloc on this port")
	shhgitFlag = flag.String("sshgit-config", "shhgit.yaml", "path to shhgit config")

	ckS = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	suS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	erS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000"))
	faS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0099"))
	paS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
)

type Checker func(context.Context, *Config) ([]*Result, error)

func checkBox(w io.Writer, s lipgloss.Style, mark string, msg string) {
	fmt.Fprintln(w,
		ckS.Render("  [")+
			s.Render(mark)+
			ckS.Render("] ")+
			s.Render(msg))
}

func fname(i interface{}) string {
	return strings.Replace(runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name(), "main.", "", 1)
}

func personality(w io.Writer, perc int) {
	fig := ""
	desc := ""
	switch {
	case perc == 0:
		fig = figure.NewFigure("Dr. Fauci", "", true).String()
		desc = "Measured safety. YOLO FAIL!"
	case perc > 75:
		fig = figure.NewFigure("LeeRoy Jenkins", "", true).String()
		desc = "Do your thang, LeeRoy!"
	case perc > 50:
		fig = figure.NewFigure("Joan de Arc", "", true).String()
		desc = "She did WHAT?"
	case perc > 25:
		fig = figure.NewFigure("Jimmy Carter", "", true).String()
		desc = "Walking into a failed nuclear reactor? That's just crazy."
	case perc > 0:
		fig = figure.NewFigure("Allan Pollock", "", true).String()
		desc = "Borrowed a fighter jet, buzzed the Tower Bridge, and lived to tell the tale"
	}

	fmt.Fprintf(w, "\n\nYour YOLO personality:\n%s\n>> %s\n", fig, desc)

}

func printResult(w io.Writer, n string, r *Result, err error) {
	switch {
	case err != nil:
		checkBox(w, erS, "error", fmt.Sprintf("%s failed: %v", n, err))
	case r.Score == r.Max: // They really YOLO
		checkBox(w, suS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))
	case r.Score == 0: // Too good
		checkBox(w, faS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))
	case r.Score > 0:
		checkBox(w, paS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))
	default:
		checkBox(w, paS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))

	}

}

func runChecks(ctx context.Context, w io.Writer, cf *Config) (int, error) {
	score := 0
	maxScore := 0
	cf.Github = strings.Replace(cf.Github, "https://github.com/", "", 1)
	parts := strings.Split(cf.Github, "/")
	cf.Owner = parts[0]
	cf.Name = parts[1]

	fmt.Fprintf(w, "Analyzing %s %s\n", cf.Github, cf.Image)

	checkers := []Checker{
		CheckCommits,
		CheckSBOM,
		CheckPrivateKeys,
		CheckSignedImage,
		CheckReleaser,
	}

	for _, c := range checkers {
		n := fname(c)
		rs, err := c(ctx, cf)
		if err != nil {
			printResult(w, n, nil, err)
			continue
		}
		for _, r := range rs {
			if r != nil {
				score += r.Score
				maxScore += r.Max
			}
			printResult(w, n, r, err)
		}
	}

	perc := 0
	if score > 0 {
		perc = int((float64(score) / float64(maxScore)) * 100)
	}

	fmt.Fprintf(w, "\nYour score: %d out of %d (%d%%)\n", score, maxScore, perc)
	personality(w, perc)

	level := (perc / 100) * 4
	fmt.Fprintf(w, "\nYour YOLO level: %d out of %d\n", (perc/100)*4, 4)

	return level, nil
}

func showBanner(w io.Writer) {
	commit := "unknown"
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, pair := range bi.Settings {
			if pair.Key == "vcs.revision" {
				commit = pair.Value
			}
		}
	}

	fmt.Fprintln(w, suS.Render(fmt.Sprintf(`
             |
   |  |  _ \ |  _ \  _|
  \_, |\___/_|\___/\__|        v0.0-%7.7s
  ___/

`, commit)))
}

func main() {
	flag.Parse()
	showBanner(os.Stdout)

	ctx := context.Background()
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	v4c := githubv4.NewClient(httpClient)

	if *serveFlag {
		addr := fmt.Sprintf(":%s", os.Getenv("PORT"))
		if addr == ":" {
			addr = fmt.Sprintf(":%d", *portFlag)
		}
		serve(ctx, &ServerConfig{Addr: addr, V4Client: v4c})
	}

	cf := &Config{
		Github:   *repoFlag,
		Image:    *imageFlag,
		V4Client: v4c,
	}

	level, err := runChecks(ctx, os.Stdout, cf)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	os.Exit(level)
}
