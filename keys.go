package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	shhgit "github.com/eth0izzle/shhgit/core"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
		opts := &git.CloneOptions{
			URL:               fmt.Sprintf("https://github.com/%s.git", c.Github),
			SingleBranch:      true,
			Depth:             1,
			RecurseSubmodules: git.NoRecurseSubmodules,
			ReferenceName:     plumbing.NewBranchReferenceName(c.Branch),
		}

		if _, err := git.PlainCloneContext(ctx, dest, false, opts); err != nil {
			log.Printf("error: %v", err)
			opts.ReferenceName = plumbing.NewBranchReferenceName("master")
			if _, err := git.PlainCloneContext(ctx, dest, false, opts); err != nil {
				return nil, fmt.Errorf("clone: %w", err)
			}
		}
	}

	res := &Result{
		Score: 0,
		Max:   10,
		Msg:   fmt.Sprintf("Zero private keys checked into %s. Sharing is caring :(", c.Github),
		Level: 2,
	}

	found, err := runShhGit(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("shhgit: %w", err)
	}

	keys := []string{}
	images := map[string]bool{}

	for _, f := range found {
		if f.kind == "image" {
			//		klog.Infof("POSSIBLE IMAGE: %s", f.content)
			if strings.Contains(f.content, c.Name) || strings.Contains(f.content, c.Owner) {
				images[f.content] = true
			}
		}

		if f.kind == "key" && !strings.Contains(f.path, "test") {
			keys = append(keys, f.path)
		}
	}

	//	log.Printf("possible images: %v", images)
	if len(keys) > 0 {
		res = &Result{
			Score: 10,
			Max:   10,
			Msg:   fmt.Sprintf("Found %d possibly private key(s): %v", len(keys), keys),
			Level: 2,
		}
	}

	for i := range images {
		c.FoundImages = append(c.FoundImages, i)
		// Add some variations
		if !strings.Contains(i, fmt.Sprintf("%s/%s", c.Name, c.Name)) {
			c.FoundImages = append(c.FoundImages, i+"/"+c.Name)
		}
		try := fmt.Sprintf("%s-server", c.Name)
		if !strings.Contains(i, try) {
			c.FoundImages = append(c.FoundImages, i+"/"+try)
		}

		try = fmt.Sprintf("%s-cli", c.Name)
		if !strings.Contains(i, try) {
			c.FoundImages = append(c.FoundImages, i+"/"+try)
		}

		try = fmt.Sprintf("%s-client", c.Name)
		if !strings.Contains(i, try) {
			c.FoundImages = append(c.FoundImages, i+"/"+try)
		}
	}

	return []*Result{res}, nil
}

type match struct {
	kind    string
	path    string
	name    string
	content string
}

func runShhGit(ctx context.Context, dir string) ([]match, error) {
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

	found := []match{}
	for _, file := range shhgit.GetMatchingFiles(s, dir) {
		relPath := strings.ReplaceAll(file.Path, dir, "")
		for _, signature := range s.Signatures {
			if matched, part := signature.Match(file); matched {
				if part == shhgit.PartContents {
					if matches := signature.StringSubMatches(s, file.Contents); len(matches) > 0 {
						if strings.HasPrefix(signature.Name(), "_IMAGE_") {
							found = append(found, match{kind: "image", path: strings.TrimLeft(relPath, "/"), name: signature.Name(), content: matches[0][1]})
							break
						}
						found = append(found, match{kind: "key", path: strings.TrimLeft(relPath, "/"), name: signature.Name()})
					}
				} else {
					found = append(found, match{kind: "key", path: strings.TrimLeft(relPath, "/"), name: signature.Name()})
				}
			}
		}

		// find a docker container?
	}

	return found, nil
}
