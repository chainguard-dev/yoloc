package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
)

type Config struct {
	Github string
	Image  string
}

type Result struct {
	Score int
	Max   int
	Msg   string
}

func CheckRoot(_ context.Context, _ *Config) (*Result, error) {
	max := 10
	if euid := os.Geteuid(); euid > 0 {
		return &Result{Score: 0, Max: max, Msg: fmt.Sprintf("effective euid is %d, not 0", euid)}, nil
	}
	return &Result{Score: max, Max: max, Msg: "I AM GROOT"}, nil
}

func pageRE(url, regex string) (bool, error) {
	resp, err := http.Get(url)
	if err != nil {
		return false, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()
	bs, err := ioutil.ReadAll(resp.Body)

	return regexp.MustCompile(regex).MatchString(string(bs)), nil
}

func CheckSBOM(_ context.Context, c *Config) (*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
	}

	ok, err := pageRE(fmt.Sprintf("https://github.com/%s", c.Github), ("(?i)sbom|spdx"))
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	if ok {
		r.Msg += "Found SBOM mention on main page. "
		r.Score = r.Score - 4
	}

	ok, err = pageRE(fmt.Sprintf("https://github.com/%s", c.Github), ("(?i)sbom|spdx"))
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	if ok {
		r.Msg += "Found SBOM mention on releases page. "
		r.Score = r.Score - 6
	}

	if r.Msg == "" {
		r.Msg = fmt.Sprintf("No SBOM found at %s", c.Github)
	}

	return r, nil
}
