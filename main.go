package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
)

var (
	repoFlag  = flag.String("repo", "google/triage-party", "Github repo to check")
	imageFlag = flag.String("image", "", "image to chec")

	ckS = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	suS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	erS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000"))
	faS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0099"))
	paS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
)

type Checker func(context.Context, *Config) (*Result, error)

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
	fmt.Printf("Your YOLO stance:\n\n")

	switch {
	case perc == 0:
		figure.NewFigure("Dr. Fauci", "", true).Print()
		fmt.Println("\nMeasured safety. YOLO FAIL!")
	case perc > 0:
		figure.NewFigure("Allan Pollock", "", true).Print()
		fmt.Println("\nBorrowed a fighter jet, buzzed the Tower Bridge, and lived to tell the tale")
	case perc == 100:
		figure.NewFigure("LeeRoy Jenkins", "", true).Print()
		fmt.Println("")
	}
}

func main() {
	flag.Parse()

	fmt.Println(suS.Render(`
             |
   |  |  _ \ |  _ \  _|
  \_, |\___/_|\___/\__|        v0.0-main
  ___/

`))

	cf := &Config{
		Github: strings.Replace(*repoFlag, "https://github.com/", "", 1),
		Image:  *imageFlag,
	}

	fmt.Printf("Analyzing %+v\n", cf)

	checkers := []Checker{
		CheckRoot,
		CheckSBOM,
		CheckSignedImage,
	}
	ctx := context.Background()
	score := 0
	maxScore := 0

	for _, c := range checkers {
		r, err := c(ctx, cf)
		if r != nil {
			score += r.Score
			maxScore += r.Max
		}
		n := fname(c)

		//fmt.Printf("%s: %+v\n", n, r)
		switch {
		case err != nil:
			checkBox(erS, "‽", fmt.Sprintf("%s failed: %v", n, err))
		case r.Score == r.Max: // They really YOLO
			checkBox(suS, "✓", fmt.Sprintf("%s: %s", n, r.Msg))
		case r.Score == 0: // Too good
			checkBox(faS, "𐄂", fmt.Sprintf("%s: %s", n, r.Msg))
		case r.Score > 0:
			checkBox(paS, "-", fmt.Sprintf("%s: %s", n, r.Msg))
		default:
			checkBox(paS, "?", fmt.Sprintf("%s: %s", n, r.Msg))

		}
	}

	perc := 0
	if score > 0 {
		perc = int((float64(score) / float64(maxScore)) * 100)
	}

	fmt.Printf("\nYour score: %d out of %d (%d%%)\n", score, maxScore, perc)
	stance(perc)
	os.Exit(perc)
}
