package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shurcooL/githubv4"
)

// Code appropriated from https://github.com/ossf/scorecard

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
						CommittedDate githubv4.DateTime
						Oid           githubv4.GitObjectID
						Author        struct {
							User struct {
								Login githubv4.String
							}
						}
						Committer struct {
							Name *string
							User struct {
								Login *string
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
								Number     githubv4.Int
								HeadRefOid githubv4.String
								MergedAt   githubv4.DateTime
								Reviews    struct {
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

type RepoAssociation int32

const (
	RepoAssociationCollaborator RepoAssociation = iota
	RepoAssociationContributor
	RepoAssociationFirstTimer
	RepoAssociationFirstTimeContributor
	RepoAssociationMannequin
	RepoAssociationMember
	RepoAssociationNone
	RepoAssociationOwner
)

func Commits(client *githubv4.Client, repoOwner, repoName string, branch string) ([]Commit, error) {
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

	ret := []Commit{}

	for {
		//log.Printf("making query: %+v\nvars: %v\n", query, vars)
		err := client.Query(context.Background(), &query, vars)
		if err != nil {
			return nil, fmt.Errorf("query: %w", err)
		}

		for _, commit := range query.Repository.Object.Commit.History.Nodes {
			var committer string
			if commit.Committer.User.Login != nil && *commit.Committer.User.Login != "" {
				committer = *commit.Committer.User.Login
			} else if commit.Committer.Name != nil &&
				*commit.Committer.Name == "GitHub" &&
				commit.Signature.IsValid &&
				commit.Signature.WasSignedByGitHub {
				committer = "github"
			}

			var associatedPR PullRequest

			approved := false

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
				if string(pr.Author.Login) != string(commit.Author.User.Login) {
					//	log.Printf("#%d: tacit approval: pr owned by %s, commit owned by %s\ncommit: %+v\npr: %+v\n", pr.Number, pr.Author.Login, commit.Author.User.Login, commit, pr)
					approved = true
				}

				for _, review := range pr.Reviews.Nodes {
					associatedPR.Reviews = append(associatedPR.Reviews, Review{
						State:  string(review.State),
						Author: &User{Login: string(review.Author.Login)},
					})
					if review.State == "APPROVED" {
						//	log.Printf("#%d: found approval: %v", pr.Number, review)
						approved = true
					}

				}
				break
			}

			if !approved {
				log.Printf("found unapproved commit: %+v\nassociated PR: %+v", commit, associatedPR)
			}

			ret = append(ret, Commit{
				CommittedDate:          commit.CommittedDate.Time,
				SHA:                    string(commit.Oid),
				Committer:              User{Login: committer},
				AssociatedMergeRequest: associatedPR,
				Signed:                 commit.Signature.IsValid,
				Approved:               approved,
			})
		}

		if len(ret) > 100 {
			break
		}

		if !query.Repository.Object.Commit.History.PageInfo.HasNextPage {
			break
		}
		vars["commitsCursor"] = githubv4.NewString(query.Repository.Object.Commit.History.PageInfo.EndCursor)
	}
	return ret, nil
}
