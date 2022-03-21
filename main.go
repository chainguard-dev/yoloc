package main

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	ckS = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	suS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	erS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0000"))
	faS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF0099"))
	paS = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
)

type Result struct {
	Score    int
	MaxScore int
	Text     string
}

type Config struct {
}

type Checker func(context.Context, *Config) (*Result, error)

func CheckRoot(_ context.Context, _ *Config) (*Result, error) {
	maxScore := 10
	if euid := os.Geteuid(); euid > 0 {
		return &Result{0, maxScore, fmt.Sprintf("effective euid is %d, not 0", euid)}, nil
	}
	return &Result{maxScore, maxScore, "I AM GROOT"}, nil
}

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

func main() {

	cf := &Config{}
	checkers := []Checker{
		CheckRoot,
	}
	ctx := context.Background()
	score := 0
	maxScore := 0

	for _, c := range checkers {
		r, err := c(ctx, cf)
		score += r.Score
		maxScore += r.MaxScore

		switch {
		case err != nil:
			checkBox(erS, "🚫", fmt.Sprintf("check %v failed: %v", c, err))
		case r.Score == r.MaxScore:
			checkBox(suS, "🚫", r.Text)
		case r.Score == 0:
			checkBox(faS, "🚫", r.Text)
		case r.Score == 0:
			checkBox(paS, "🚫", r.Text)
		}
	}

	perc := int(score/maxScore) * 100
	fmt.Printf("\nYour score: %d out of %d (%d%%)\n", score, maxScore, perc)
	os.Exit(perc)
}
