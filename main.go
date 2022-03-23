package main

import (
	"context"
	"flag"
	"fmt"
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
	repoFlag  = flag.String("repo", "google/triage-party", "Github repo to check")
	imageFlag = flag.String("image", "", "image to check")
	serveFlag = flag.Bool("serve", false, "yoloc webserver mode")
	portFlag  = flag.Int("port", 8080, "serve yoloc on this port")

	ckS = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	suS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	erS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000"))
	faS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0099"))
	paS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
)

type Checker func(context.Context, *Config) ([]*Result, error)

func out(s lipgloss.Style, msg string, args ...interface{}) {
	fmt.Printf(s.Render(fmt.Sprintf(msg, args...)))
}

func checkBox(s lipgloss.Style, mark string, msg string) {
	fmt.Println(
		ckS.Render("  [") +
			s.Render(mark) +
			ckS.Render("] ") +
			s.Render(msg))
}

func fname(i interface{}) string {
	return strings.Replace(runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name(), "main.", "", 1)
}

func stance(perc int) {
	fmt.Printf("Your YOLO personality:\n\n")

	switch {
	case perc == 0:
		figure.NewFigure("Dr. Fauci", "", true).Print()
		fmt.Println("\nMeasured safety. YOLO FAIL!")
	case perc > 75:
		figure.NewFigure("LeeRoy Jenkins", "", true).Print()
		fmt.Println("")
	case perc > 50:
		figure.NewFigure("Joan de Arc", "", true).Print()
		fmt.Println("\nShe did WHAT?")
	case perc > 25:
		figure.NewFigure("Jimmy Carter", "", true).Print()
		fmt.Println("\nCrazy.")
	case perc > 0:
		figure.NewFigure("Allan Pollock", "", true).Print()
		fmt.Println("\nBorrowed a fighter jet, buzzed the Tower Bridge, and lived to tell the tale")
	}
}

func printResult(n string, r *Result, err error) {
	switch {
	case err != nil:
		checkBox(erS, "error", fmt.Sprintf("%s failed: %v", n, err))
	case r.Score == r.Max: // They really YOLO
		checkBox(suS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))
	case r.Score == 0: // Too good
		checkBox(faS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))
	case r.Score > 0:
		checkBox(paS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))
	default:
		checkBox(paS, fmt.Sprintf("%2d/%2d", r.Score, r.Max), fmt.Sprintf("%s: %s", n, r.Msg))

	}

}

func runChecks(ctx context.Context, cf *Config) (int, error) {
	score := 0
	maxScore := 0

	fmt.Printf("Analyzing %s %s\n", cf.Github, cf.Image)

	checkers := []Checker{
		CheckCommits,
		CheckSBOM,
		CheckSignedImage,
		CheckReleaser,
	}

	for _, c := range checkers {
		n := fname(c)
		rs, err := c(ctx, cf)
		for _, r := range rs {
			if r != nil {
				score += r.Score
				maxScore += r.Max
			}
			printResult(n, r, err)
		}
	}

	perc := 0
	if score > 0 {
		perc = int((float64(score) / float64(maxScore)) * 100)
	}

	fmt.Printf("\nYour score: %d out of %d (%d%%)\n", score, maxScore, perc)
	stance(perc)

	level := (perc / 100) * 4
	fmt.Printf("\nYour YOLO level: %d out of %d\n", (perc/100)*4, 4)

	return level, nil
}

func main() {
	flag.Parse()
	commit := "unknown"
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, pair := range bi.Settings {
			if pair.Key == "vcs.revision" {
				commit = pair.Value
			}
		}
	}

	fmt.Println(suS.Render(fmt.Sprintf(`
             |
   |  |  _ \ |  _ \  _|
  \_, |\___/_|\___/\__|        v0.0-%7.7s
  ___/

`, commit)))

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

	repo := strings.Replace(*repoFlag, "https://github.com/", "", 1)
	parts := strings.Split(repo, "/")

	cf := &Config{
		Github:   repo,
		Owner:    parts[0],
		Name:     parts[1],
		Image:    *imageFlag,
		V4Client: v4c,
	}

	level, err := runChecks(ctx, cf)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	os.Exit(level)
}
