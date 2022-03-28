package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/common-nighthawk/go-figure"
	au "github.com/logrusorgru/aurora"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

var (
	repoFlag   = flag.String("repo", "sigstore/fulcio", "Github repo to check")
	imageFlag  = flag.String("image", "", "image to check")
	serveFlag  = flag.Bool("serve", false, "yoloc webserver mode")
	portFlag   = flag.Int("port", 8080, "serve yoloc on this port")
	shhgitFlag = flag.String("sshgit-config", "shhgit.yaml", "path to shhgit config")
)

type (
	Checker   func(context.Context, *Config) ([]*Result, error)
	Colorizer func(arc interface{}) au.Value
)

func checkBox(w io.Writer, c Colorizer, mark string, msg string) {
	fmt.Fprintln(w,
		au.BrightBlack("  ["),
		c(mark),
		au.BrightBlack("] "),
		c(msg))
}

func fname(i interface{}) string {
	return strings.Replace(runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name(), "main.", "", 1)
}

func personality(w io.Writer, perc int) {
	fig := ""
	desc := ""
	color := au.White
	switch {
	case perc == 0:
		fig = figure.NewFigure("Dr. Fauci", "", true).String()
		desc = "Measured safety. YOLO FAIL!"
	case perc > 75:
		color = au.BrightGreen
		fig = figure.NewFigure("LeeRoy Jenkins", "", true).String()
		desc = "Do your thang, LeeRoy!"
	case perc > 60:
		color = au.BrightRed
		fig = figure.NewFigure("Allan Pollock", "", true).String()
		desc = "Borrowed a fighter jet, buzzed the Tower Bridge, and lived to tell the tale."
	case perc > 40:
		color = au.BrightYellow
		fig = figure.NewFigure("Joan of Arc", "", true).String()
		desc = "Led France into battle without a sword."
	case perc > 20:
		color = au.BrightMagenta
		fig = figure.NewFigure("Jimmy Carter", "", true).String()
		desc = "Walking into a failed nuclear reactor? That's just crazy."
	case perc > 0:
		color = au.BrightRed
		fig = figure.NewFigure("W. Jennings Bryan", "", true).String()
		desc = "Doesn't drink. Doesn't smoke. Doesn't chew. Doesn't swear. Ran for president multiple times."
	}

	fmt.Fprintf(w, "Your YOLO personality:\n%s\n>> %s\n", color(fig), desc)
}

func badge(w io.Writer, level int) {
	color := ""
	switch level {
	case 0:
		color = "brightgreen"
	case 1:
		color = "green"
	case 2:
		color = "yellow"
	case 3:
		color = "orange"
	case 4:
		color = "red"
	}

	badge := fmt.Sprintf("https://img.shields.io/badge/YOLO-%d-%s", level, color)
	fmt.Fprintf(w, "\nTo add this badge to a GitHub README.md:\n[![YOLO Level](%s)](https://yolo.tools)\n\n", badge)
}

func printResult(w io.Writer, n string, r *Result, err error) {
	switch {
	case err != nil:
		checkBox(w, au.BrightRed, "error", fmt.Sprintf("%s failed: %v", n, err))
	case r.Score == r.Max: // They really YOLO
		checkBox(w, au.BrightGreen, fmt.Sprintf("%2d/%2d", r.Score, r.Max), r.Msg)
	case r.Score == 0: // Too good
		checkBox(w, au.Magenta, fmt.Sprintf("%2d/%2d", r.Score, r.Max), r.Msg)
	case r.Score > 0:
		checkBox(w, au.BrightYellow, fmt.Sprintf("%2d/%2d", r.Score, r.Max), r.Msg)
	default:
		checkBox(w, au.White, fmt.Sprintf("%2d/%2d", r.Score, r.Max), r.Msg)
	}
}

func runChecks(ctx context.Context, w io.Writer, cf *Config) int {
	score := 0
	maxScore := 0
	cf.Github = strings.Replace(cf.Github, "https://github.com/", "", 1)
	parts := strings.Split(cf.Github, "/")
	cf.Owner = parts[0]
	cf.Name = parts[1]

	fmt.Fprintf(w, "Analyzing %s %s ...\n\n", cf.Github, cf.Image)

	checkers := []Checker{
		CheckSBOM,
		CheckReleaser,
		CheckCommits,
		CheckPrivateKeys,
		CheckSignedImage,
	}

	maxLevel := 0

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
				// For fun, we assign your level to be the highest observed
				if r.Score > 0 && r.Level > maxLevel {
					maxLevel = r.Level
				}
			}
			printResult(w, n, r, err)
		}
	}

	perc := 0
	if score > 0 {
		perc = int((float64(score) / float64(maxScore)) * 100)
	}

	fmt.Fprintf(w, "\nYour YOLO score: %d out of %d (%d%%)\n", score, maxScore, perc)
	personality(w, perc)

	fmt.Fprintf(w, "\nYour YOLO compliance level: %d\n", maxLevel)

	badge(w, maxLevel)

	level := (perc / 100) * 4
	return level
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

	fmt.Fprintln(w, au.BrightGreen(fmt.Sprintf(`
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

	level := runChecks(ctx, os.Stdout, cf)
	os.Exit(level)
}
