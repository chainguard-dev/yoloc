package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	shhgit "github.com/eth0izzle/shhgit/core"
	"github.com/go-git/go-git/v5"
)

func CheckPrivateKeys(ctx context.Context, c *Config) ([]*Result, error) {
	cd, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}

	dest := filepath.Join(cd, "yoloc", c.Owner, c.Name)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		_, err := git.PlainOpen(dest)
		if err != nil {
			return nil, fmt.Errorf("clone: %w", err)
		}
	} else {
		_, err := git.PlainCloneContext(ctx, dest, false, &git.CloneOptions{
			URL:               fmt.Sprintf("https://github.com/%s.git", c.Github),
			SingleBranch:      true,
			Depth:             1,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
		if err != nil {
			return nil, fmt.Errorf("clone: %w", err)
		}

	}

	res := &Result{
		Score: 0,
		Max:   10,
		Msg:   "Found zero private keys",
	}

	found, err := runShhGit(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("shhgit: %w", err)
	}

	if len(found) > 0 {
		res = &Result{
			Score: 10,
			Max:   10,
			Msg:   fmt.Sprintf("Found %d paths that resemble a private keys: %v", len(found), found),
		}
	}
	return []*Result{res}, nil
}

func runShhGit(ctx context.Context, dir string) ([]string, error) {
	maxSize := uint(16)
	koData := os.Getenv("KO_DATA_PATH")
	if koData == "" {
		koData = "kodata/"
	}

	s, err := shhgit.NewSession(ctx, &shhgit.Options{
		Local:           &dir,
		MaximumFileSize: &maxSize,
		ConfigName:      shhgitFlag,
		ConfigPath:      &koData,
	})
	if err != nil {
		return nil, fmt.Errorf("shhgit: %w", err)
	}

	found := []string{}
	for _, file := range shhgit.GetMatchingFiles(s, dir) {
		relPath := strings.Replace(file.Path, dir, "", -1)
		for _, signature := range s.Signatures {
			if matched, part := signature.Match(file); matched {
				if part == shhgit.PartContents {
					if matches := signature.GetContentsMatches(s, file.Contents); len(matches) > 0 {
						found = append(found, strings.TrimLeft(relPath, "/"))
					}
				}
			}
		}
	}

	return found, nil
}
