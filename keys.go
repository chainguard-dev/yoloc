package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	shhgit "github.com/eth0izzle/shhgit/core"
	"github.com/go-git/go-git/v5"
	"k8s.io/klog/v2"
)

func CheckPrivateKeys(ctx context.Context, c *Config) ([]*Result, error) {
	cd, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}

	dest := filepath.Join(cd, "yoloc", c.Owner, c.Name)
	klog.Infof("dest: %s", dest)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}

	var r *git.Repository
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		r, err = git.PlainOpen(dest)
		if err != nil {
			return nil, fmt.Errorf("clone: %w", err)
		}
	} else {
		r, err = git.PlainCloneContext(ctx, dest, false, &git.CloneOptions{
			URL:               fmt.Sprintf("https://github.com/%s.git", c.Github),
			SingleBranch:      true,
			Depth:             1,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
		if err != nil {
			return nil, fmt.Errorf("clone: %w", err)
		}

	}

	_, err = r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("wt: %w", err)
	}

	ref, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}
	klog.Infof("ref: %+v", ref)

	res := &Result{
		Score: 0,
		Max:   10,
		Msg:   "Found zero private keys",
	}

	found, err := runShhGit(dest)
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

func runShhGit(dir string) ([]string, error) {
	maxSize := uint(16)
	koData := os.Getenv("KO_DATA_PATH")

	s, err := shhgit.NewSession(&shhgit.Options{
		Local:           &dir,
		MaximumFileSize: &maxSize,
		ConfigName:      shhgitFlag,
		ConfigPath:      &koData,
	})
	if err != nil {
		return nil, fmt.Errorf("shhgit: %w", err)
	}

	found := []string{}
	for _, file := range shhgit.GetMatchingFiles(dir) {
		relPath := strings.Replace(file.Path, dir, "", -1)
		for _, signature := range s.Signatures {
			if matched, part := signature.Match(file); matched {
				if part == shhgit.PartContents {
					if matches := signature.GetContentsMatches(file.Contents); len(matches) > 0 {
						found = append(found, strings.TrimLeft(relPath, "/"))
					}
				}
			}
		}
	}

	return found, nil
}
