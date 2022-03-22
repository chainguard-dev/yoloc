package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/cmd/cosign/cli/fulcio"
	"github.com/sigstore/cosign/pkg/cosign"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	rekor "github.com/sigstore/rekor/pkg/client"
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

	ok, err = pageRE(fmt.Sprintf("https://github.com/%s/releases", c.Github), ("(?i)sbom|spdx"))
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

func CheckSignedImage(_ context.Context, c *Config) (*Result, error) {
	// Does not yet implement autodiscovery
	if c.Image == "" {
		return nil, fmt.Errorf("Image URL not provided")
	}

	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 0 verified signatures",
	}

	ctx := context.TODO()
	ref, err := name.ParseReference(c.Image)
	if err != nil {
		return nil, fmt.Errorf("parse ref: %w", err)
	}

	opts := []remote.Option{
		remote.WithContext(ctx),
	}

	rc, err := rekor.GetRekorClient("https://api.sigstore.dev")
	if err != nil {
		return nil, fmt.Errorf("rekor: %w", err)
	}

	co := &cosign.CheckOpts{
		ClaimVerifier:      cosign.SimpleClaimVerifier,
		RegistryClientOpts: []ociremote.Option{ociremote.WithRemoteOptions(opts...)},
		RekorClient:        rc,
		RootCerts:          fulcio.GetRoots(),
		//	SigVerifier:        pubKey,
	}

	vs, _, err := cosign.VerifyImageSignatures(ctx, ref, co)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	if len(vs) > 0 {
		r.Score = 0
		r.Msg = fmt.Sprintf("Found %d verified signatures", len(vs))
	}

	return r, nil
}

func CheckApprovers(_ context.Context, c *Config) (*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "100% of commits have 0 approvers",
	}
	return r, nil
}

func CheckPrivateKeys(_ context.Context, c *Config) (*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 1 things that look like private keys",
	}
	return r, nil
}

func CheckDependencies(_ context.Context, c *Config) (*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 1 things that look like private keys",
	}
	return r, nil
}

func CheckReproducibleBuild(_ context.Context, c *Config) (*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 1 things that look like private keys",
	}
	return r, nil
}

func CheckReleaser(_ context.Context, c *Config) (*Result, error) {
	url := fmt.Sprintf("https://github.com/%s/releases", c.Github)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()
	bs, err := ioutil.ReadAll(resp.Body)

	matches := regexp.MustCompile(`data-hovercard-url="/users/(.*?)/hovercard`).FindStringSubmatch(string(bs))
	if len(matches) == 0 {
		return &Result{Score: 10, Max: 10, Msg: fmt.Sprintf("No releases found. Great work!")}, nil
	}

	user := matches[1]
	if regexp.MustCompile("bot|action|release|jenkins|auto").MatchString(user) {
		return &Result{Score: 0, Max: 10, Msg: fmt.Sprintf("Previous release was created by automation (%q)", user)}, nil
	}

	return &Result{Score: 4, Max: 10, Msg: fmt.Sprintf("Releases found, last by %s (not automated)", user)}, nil
}

func CheckArtifactSignatures(_ context.Context, c *Config) (*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 1 things that look like private keys",
	}
	return r, nil
}
