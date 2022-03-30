package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	lru "github.com/hnlq715/golang-lru"
	"github.com/shurcooL/githubv4"
)

var (
	maxCommits   = 200
	maxCommitAge = 365 * 24 * time.Hour
)

// Based heavily on code from https://github.com/ossf/scorecard - thanks guys!

type graphqlData struct {
	Repository struct {
		Object struct {
			Commit struct {
				History struct {

					// PageInfo is used for pagination
					PageInfo struct {
						EndCursor   githubv4.String
						HasNextPage bool
					}
					Nodes []struct {
						AuthoredByCommitter bool
						CommittedDate       githubv4.DateTime
						Oid                 githubv4.GitObjectID
						Author              struct {
							User struct {
								Login githubv4.String
							}
						}
						Committer struct {
							Name githubv4.String
							User struct {
								Login githubv4.String
							}
						}
						Signature struct {
							IsValid           bool
							WasSignedByGitHub bool
						}
						AssociatedPullRequests struct {
							Nodes []struct {
								Repository struct {
									Name  githubv4.String
									Owner struct {
										Login githubv4.String
									}
								}
								Author struct {
									Login githubv4.String
								}
								AuthorAssociation githubv4.String
								Number            githubv4.Int
								HeadRefOid        githubv4.String
								MergeCommit       struct {
									Author struct {
										User struct {
											Login githubv4.String
										}
									}
								}
								MergedAt githubv4.DateTime
								MergedBy struct {
									Login githubv4.String
								}
								Reviews struct {
									Nodes []struct {
										State  githubv4.String
										Author struct {
											Login githubv4.String
										}
									}
								} `graphql:"reviews(last: $reviewsToAnalyze)"`
							}
						} `graphql:"associatedPullRequests(first: $pullRequestsToAnalyze)"`
					}
					// the "after" construct enables pagination.
				} `graphql:"history(first: $commitsToAnalyze, after: $commitsCursor)"`
			} `graphql:"... on Commit"`
		} `graphql:"object(expression: $expression)"` // Only use commits from specified branch
	} `graphql:"repository(owner: $owner, name: $name)"`
}

type Commit struct {
	CommittedDate          time.Time
	Message                string
	SHA                    string
	Committer              User
	Signed                 bool
	Approved               bool
	Reviewed               bool
	AssociatedMergeRequest PullRequest
}

type PullRequest struct {
	Number   int
	MergedAt time.Time
	HeadSHA  string
	Author   User
	Labels   []Label
	Reviews  []Review
}

type Label struct {
	Name string
}

type Review struct {
	Author *User
	State  string
}

type User struct {
	Login string
}

func asSha256(o interface{}) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", o)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

func Commits(client *githubv4.Client, repoOwner, repoName string, branch string, l *lru.ARCCache) ([]Commit, error) {
	query := &graphqlData{}
	vars := map[string]interface{}{
		"owner":                 githubv4.String(repoOwner),
		"name":                  githubv4.String(repoName),
		"pullRequestsToAnalyze": githubv4.Int(10),
		"commitsToAnalyze":      githubv4.Int(100),
		"reviewsToAnalyze":      githubv4.Int(10),
		"expression":            githubv4.String(branch),
		"commitsCursor":         (*githubv4.String)(nil),
	}

	ageCutoff := time.Now().Add(maxCommitAge * -1)
	ret := []Commit{}

	varHashed := asSha256(vars)
	cached, exist := l.Get(varHashed)
	if exist {
		fmt.Println("cached yayyy")
		return cached.([]Commit), nil
	}

	for {
		err := client.Query(context.Background(), &query, vars)
		if err != nil {
			return nil, fmt.Errorf("query: %w", err)
		}

		for _, commit := range query.Repository.Object.Commit.History.Nodes {
			var committer string
			if commit.Committer.User.Login != "" {
				committer = string(commit.Committer.User.Login)
			} else if commit.Committer.Name != "" &&
				commit.Committer.Name == "GitHub" &&
				commit.Signature.IsValid &&
				commit.Signature.WasSignedByGitHub {
				committer = "github"
			}

			var associatedPR PullRequest

			approved := false
			reviewed := false

			for _, pr := range commit.AssociatedPullRequests.Nodes {
				if string(pr.Repository.Owner.Login) != repoOwner ||
					string(pr.Repository.Name) != repoName {
					continue
				}
				associatedPR = PullRequest{
					Number:   int(pr.Number),
					HeadSHA:  string(pr.HeadRefOid),
					MergedAt: pr.MergedAt.Time,
					Author:   User{Login: string(pr.Author.Login)},
				}

				// Merging someone elses PR is considered tacit approval
				if string(pr.MergedBy.Login) != string(commit.Author.User.Login) {
					// log.Printf("#%d: tacit approval: pr merged by %s, commit owned by %s\ncommit: %+v\npr: %+v\n", pr.Number, pr.MergedBy.Login, commit.Author.User.Login, commit, pr)
					approved = true
					reviewed = true
				}

				for _, review := range pr.Reviews.Nodes {
					associatedPR.Reviews = append(associatedPR.Reviews, Review{
						State:  string(review.State),
						Author: &User{Login: string(review.Author.Login)},
					})

					if review.Author.Login != pr.Author.Login {
						reviewed = true
					}

					if review.State == "APPROVED" {
						//	log.Printf("#%d: found approval: %v", pr.Number, review)
						approved = true
					}
				}
				break
			}

			//			if !reviewed {
			//				log.Printf("\n--------\nunreviewed commit: %+v\n--------\n", commit)
			//			}

			ret = append(ret, Commit{
				CommittedDate:          commit.CommittedDate.Time,
				SHA:                    string(commit.Oid),
				Committer:              User{Login: committer},
				AssociatedMergeRequest: associatedPR,
				Signed:                 commit.Signature.IsValid,
				Approved:               approved,
				Reviewed:               reviewed,
			})

			if commit.CommittedDate.Before(ageCutoff) {
				break
			}
		}

		if len(ret) > maxCommits {
			break
		}

		if !query.Repository.Object.Commit.History.PageInfo.HasNextPage {
			break
		}
		vars["commitsCursor"] = githubv4.NewString(query.Repository.Object.Commit.History.PageInfo.EndCursor)
		l.Add(varHashed, ret)
	}

	return ret, nil
}
