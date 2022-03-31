package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/go-version"
	lru "github.com/hnlq715/golang-lru"
	"github.com/shurcooL/githubv4"
	"k8s.io/klog/v2"

	"github.com/sigstore/cosign/cmd/cosign/cli/fulcio"
	"github.com/sigstore/cosign/pkg/cosign"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	rekor "github.com/sigstore/rekor/pkg/client"
)

type Config struct {
	Github      string
	Image       string
	V4Client    *githubv4.Client
	Cache       *lru.ARCCache
	Owner       string
	Name        string
	Branch      string
	FoundImages []string
	Persist     Persister
}

type Result struct {
	Score int
	Max   int
	Msg   string
	Level int
}

func pageRE(ctx context.Context, url, regex string) (bool, error) {
	bs, err := getCtx(ctx, url)
	if err != nil {
		return false, fmt.Errorf("get: %w", err)
	}
	return regexp.MustCompile(regex).MatchString(string(bs)), nil
}

func CheckSBOM(ctx context.Context, c *Config) ([]Result, error) {
	res := []Result{}
	level := 1

	ok, err := pageRE(ctx, fmt.Sprintf("https://github.com/%s", c.Github), ("(?i)sbom|spdx"))
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}

	found := []string{}

	if ok {
		found = append(found, "main page")
	}

	ok, err = pageRE(ctx, fmt.Sprintf("https://github.com/%s/releases", c.Github), ("(?i)sbom|spdx"))
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	if ok {
		found = append(found, "releases page")
	}

	if len(found) == 0 {
		res = append(res, Result{Msg: "No evidence of SBOM usage on main or releases page", Score: 10, Max: 10, Level: level})
	} else {
		res = append(res, Result{Msg: fmt.Sprintf("Found evidence of SBOM usage on %s", strings.Join(found, ", ")), Score: 0, Max: 10, Level: level})

	}

	return res, nil
}

func pickTagToAnalyze(vs []string) string {
	versions := []*version.Version{}
	seen := map[string]bool{}
	for _, raw := range vs {
		seen[raw] = true
		if strings.Contains(raw, ".") {
			v, err := version.NewVersion(raw)
			if err != nil {
				//		klog.Errorf("version err: %v", err)
				continue
			}
			versions = append(versions, v)
		}
	}

	for _, tag := range []string{"latest", "stable"} {
		if seen[tag] {
			return tag
		}
	}

	if len(versions) > 0 {
		//klog.Infof("found versions: %v", versions)
		sort.Sort(version.Collection(versions))
		return versions[len(versions)-1].Original()
	}

	// I give up
	return vs[0]
}

func CheckSignedImage(ctx context.Context, c *Config) ([]Result, error) {
	images := []string{}
	if c.Image != "" {
		images = append(images, c.Image)
	} else {
		images = append(images, c.FoundImages...)
	}

	if len(images) == 0 {
		return []Result{{Msg: "no image"}}, nil
	}

	res := []Result{}
	rc, err := rekor.GetRekorClient("https://api.sigstore.dev")
	if err != nil {
		return nil, fmt.Errorf("rekor: %w", err)
	}

	opts := []remote.Option{
		remote.WithContext(ctx),
	}
	co := &cosign.CheckOpts{
		ClaimVerifier:      cosign.SimpleClaimVerifier,
		RegistryClientOpts: []ociremote.Option{ociremote.WithRemoteOptions(opts...)},
		RekorClient:        rc,
		RootCerts:          fulcio.GetRoots(),
	}

	for _, ri := range images {
		//	klog.Infof("MAYBE: %s", ri)
		i := ri

		if !strings.Contains(i, ":") {
			r, err := name.NewRepository(i)
			if err != nil {
				klog.Errorf("not a repo: %v", err)
				continue
			}

			ls, err := remote.List(r, opts...)
			if err != nil {
				klog.V(1).Infof("unable to list %s: %v", i, err)
				continue
			}

			if len(ls) > 0 {
				i = fmt.Sprintf("%s:%s", ri, pickTagToAnalyze(ls))
			}
		}

		ref, err := name.ParseReference(i)
		if err != nil {
			klog.Errorf("unable to check %s: %v", i, err)
			continue
		}

		if _, err = remote.Image(ref, opts...); err != nil {
			klog.V(1).Infof("unable to check image: %v", err)
			continue
		}

		vs, _, err := cosign.VerifyImageSignatures(ctx, ref, co)
		if err != nil {
			if strings.Contains(err.Error(), "no matching signatures") {
				res = append(res, Result{Msg: fmt.Sprintf("%s is unsigned!", i), Score: 10, Max: 10, Level: 1})
			} else {
				res = append(res, Result{Msg: fmt.Sprintf("%s signature verification failure: %v", i, strings.TrimSpace(err.Error())), Score: 10, Max: 10, Level: 1})
			}
			continue
		}

		if len(vs) > 0 {
			res = append(res, Result{Msg: fmt.Sprintf("%s has a verified signature! EPIC YOLO FAIL!", i), Score: 0, Max: 10, Level: 1})
		} else {
			res = append(res, Result{Msg: fmt.Sprintf("%s has no verified signature!", i), Score: 10, Max: 10, Level: 1})
		}
	}

	return res, nil
}

func CheckCommits(_ context.Context, c *Config) ([]Result, error) {
	res := []Result{}

	signed := 0
	approved := 0
	commits := 0
	pr := 0
	reviewed := 0

	if c.Branch == "" {
		c.Branch = "main"
	}

	cs, err := Commits(c.V4Client, c.Owner, c.Name, c.Branch, c.Cache)
	if err != nil {
		return nil, fmt.Errorf("unable to get commits: %w", err)
	}

	if len(cs) == 0 {
		c.Branch = "master"
		cs, err = Commits(c.V4Client, c.Owner, c.Name, c.Branch, c.Cache)
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
	res = append(res, Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits were signed. ", percSigned*100, len(cs)), Score: 5 - int(math.Ceil(5*percSigned)), Max: 5})

	percApproved := (float64(approved) / float64(commits))
	res = append(res, Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits were approved.", percApproved*100, len(cs)), Score: 10 - int(math.Ceil(10*percApproved)), Max: 10, Level: 1})

	percReviewed := (float64(reviewed) / float64(commits))
	res = append(res, Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits were reviewed.", percReviewed*100, len(cs)), Score: 10 - int(math.Ceil(10*percReviewed)), Max: 10, Level: 1})

	percPR := (float64(pr) / float64(commits))
	res = append(res, Result{Msg: fmt.Sprintf("%.1f%% of the last %d commits had an associated PR", percPR*100, len(cs)), Score: 5 - int(math.Ceil(5*percApproved)), Max: 5, Level: 1})

	staleDays := int(time.Since(newest).Hours() / 24)
	if staleDays > 90 {
		res = append(res, Result{Msg: fmt.Sprintf("Last commit was %d days ago (abandoned)", staleDays), Score: 5, Max: 5})
	} else {
		res = append(res, Result{Msg: fmt.Sprintf("Last commit was %d days ago (active)", staleDays), Score: 0, Max: 5})
	}

	return res, nil
}

func getCtx(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func CheckReleaser(ctx context.Context, c *Config) ([]Result, error) {
	res := []Result{}
	bs, err := getCtx(ctx, fmt.Sprintf("https://github.com/%s/releases", c.Github))
	if err != nil {
		return nil, err
	}

	matches := regexp.MustCompile(`data-hovercard-url="/users/(.*?)/hovercard`).FindStringSubmatch(string(bs))
	if len(matches) == 0 {
		res = append(res, Result{Score: 10, Max: 10, Msg: "No releases found? Nice!"})
		return res, nil
	}

	if user := matches[1]; regexp.MustCompile(fmt.Sprintf("bot|action|release|jenkins|auto|%s", c.Name)).MatchString(user) {
		res = append(res, Result{Score: 0, Max: 10, Msg: fmt.Sprintf("Previous release was likely automated (%q)", user)})
	} else {
		res = append(res, Result{Score: 4, Max: 10, Msg: fmt.Sprintf("Releases found, last by %s (not automated)", user)})
	}
	return res, nil
}
