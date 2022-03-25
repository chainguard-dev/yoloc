package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/shurcooL/githubv4"
	"github.com/sigstore/cosign/cmd/cosign/cli/fulcio"
	"github.com/sigstore/cosign/pkg/cosign"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	rekor "github.com/sigstore/rekor/pkg/client"
)

type Config struct {
	Github   string
	Image    string
	V4Client *githubv4.Client
	Owner    string
	Name     string
}

type Result struct {
	Score int
	Max   int
	Msg   string
	Level int
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

func CheckSBOM(_ context.Context, c *Config) ([]*Result, error) {
	res := []*Result{}

	ok, err := pageRE(fmt.Sprintf("https://github.com/%s", c.Github), ("(?i)sbom|spdx"))
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	if ok {
		res = append(res, &Result{Msg: "Found evidence of SBOM usage on main page", Score: 0, Max: 10})
	} else {
		res = append(res, &Result{Msg: "No evidence of SBOM usage on main page", Score: 5, Max: 5})
	}

	ok, err = pageRE(fmt.Sprintf("https://github.com/%s/releases", c.Github), ("(?i)sbom|spdx"))
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	if ok {
		res = append(res, &Result{Msg: "Found evidence of SBOM usage on releases page", Score: 0, Max: 10})
	} else {
		res = append(res, &Result{Msg: "No evidence of SBOM usage on releases page", Score: 10, Max: 10})
	}

	return res, nil
}

func CheckSignedImage(_ context.Context, c *Config) ([]*Result, error) {
	// Does not yet implement autodiscovery
	if c.Image == "" {
		return nil, fmt.Errorf("Image URL not provided")
	}

	res := []*Result{}

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
		res = append(res, &Result{Msg: fmt.Sprintf("Found %d verified signatures", len(vs)), Score: 0, Max: 10})
	} else {
		res = append(res, &Result{Msg: "Found no verified signatures!", Score: 10, Max: 10})
	}

	return res, nil
}

func CheckCommits(_ context.Context, c *Config) ([]*Result, error) {
	res := []*Result{}

	signed := 0
	approved := 0
	commits := 0
	pr := 0
	reviewed := 0

	cs, err := Commits(c.V4Client, c.Owner, c.Name, "main")
	if err != nil {
		return nil, fmt.Errorf("unable to get commits: %w", err)
	}

	if len(cs) == 0 {
		cs, err = Commits(c.V4Client, c.Owner, c.Name, "master")
		if err != nil {
			return nil, fmt.Errorf("unable to get commits: %w", err)
		}
	}

	newest := time.Time{}
	for _, co := range cs {
		if co.CommittedDate.After(newest) {
			newest = co.CommittedDate
		}
		commits++
		if co.Signed {
			signed++
		}
		if co.Approved {
			approved++
		}

		if co.Reviewed {
			reviewed++
		}

		if co.AssociatedMergeRequest.Number > 0 {
			pr++
		}
	}

	percSigned := (float64(signed) / float64(commits))
	res = append(res, &Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits were signed. ", percSigned*100, len(cs)), Score: 5 - int(math.Ceil(5*percSigned)), Max: 5})

	percApproved := (float64(approved) / float64(commits))
	res = append(res, &Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits were approved.", percApproved*100, len(cs)), Score: 10 - int(math.Ceil(10*percApproved)), Max: 10})

	percReviewed := (float64(reviewed) / float64(commits))
	res = append(res, &Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits were reviewed.", percReviewed*100, len(cs)), Score: 10 - int(math.Ceil(10*percReviewed)), Max: 10})

	percPR := (float64(pr) / float64(commits))
	res = append(res, &Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits had an associated PR", percPR*100, len(cs)), Score: 5 - int(math.Ceil(5*percApproved)), Max: 5})

	staleDays := int(time.Now().Sub(newest).Hours() / 24)
	if staleDays > 90 {
		res = append(res, &Result{Msg: fmt.Sprintf("Last commit was %d days ago (abandoned)", staleDays), Score: 5, Max: 5})
	} else {
		res = append(res, &Result{Msg: fmt.Sprintf("Last commit was %d days ago (active)", staleDays), Score: 0, Max: 5})
	}

	return res, nil
}

func CheckDependencies(_ context.Context, c *Config) ([]*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 1 things that look like private keys",
	}
	return []*Result{r}, nil
}

func CheckReproducibleBuild(_ context.Context, c *Config) ([]*Result, error) {
	max := 10
	r := &Result{
		Score: max,
		Max:   max,
		Msg:   "Found 1 things that look like private keys",
	}
	return []*Result{r}, nil
}

func CheckReleaser(_ context.Context, c *Config) ([]*Result, error) {
	res := []*Result{}

	url := fmt.Sprintf("https://github.com/%s/releases", c.Github)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()
	bs, err := ioutil.ReadAll(resp.Body)

	matches := regexp.MustCompile(`data-hovercard-url="/users/(.*?)/hovercard`).FindStringSubmatch(string(bs))
	if len(matches) == 0 {
		res = append(res, &Result{Score: 10, Max: 10, Msg: fmt.Sprintf("No releases found?? Probably a bug.")})
		return res, nil
	}

	user := matches[1]
	if regexp.MustCompile("bot|action|release|jenkins|auto").MatchString(user) {
		res = append(res, &Result{Score: 0, Max: 10, Msg: fmt.Sprintf("Previous release was created by automation (%q)", user)})
	} else {
		res = append(res, &Result{Score: 4, Max: 10, Msg: fmt.Sprintf("Releases found, last by %s (not fully automated)", user)})
	}
	return res, nil
}
