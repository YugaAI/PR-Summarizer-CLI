package vcs

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v66/github"
)

const commentMarker = "<!-- pr-summarizer-bot -->"

// GitHubClient posts or updates the PR summary comment idempotently: it
// looks for an existing comment whose body starts with commentMarker and
// edits it in place, otherwise it creates a new one.
type GitHubClient struct {
	client *github.Client
	owner  string
	repo   string
}

func NewGitHubClient(token, owner, repo string) *GitHubClient {
	return &GitHubClient{
		client: github.NewClient(nil).WithAuthToken(token),
		owner:  owner,
		repo:   repo,
	}
}

func (g *GitHubClient) PostOrUpdateComment(ctx context.Context, prNumber int, body string) error {
	existing, err := g.findExistingComment(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("find existing comment: %w", err)
	}

	comment := &github.IssueComment{Body: &body}
	if existing != nil {
		if _, _, err := g.client.Issues.EditComment(ctx, g.owner, g.repo, existing.GetID(), comment); err != nil {
			return fmt.Errorf("update comment: %w", err)
		}
		return nil
	}

	if _, _, err := g.client.Issues.CreateComment(ctx, g.owner, g.repo, prNumber, comment); err != nil {
		return fmt.Errorf("create comment: %w", err)
	}
	return nil
}

func (g *GitHubClient) findExistingComment(ctx context.Context, prNumber int) (*github.IssueComment, error) {
	opts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := g.client.Issues.ListComments(ctx, g.owner, g.repo, prNumber, opts)
		if err != nil {
			return nil, err
		}
		for _, c := range comments {
			if strings.HasPrefix(c.GetBody(), commentMarker) {
				return c, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return nil, nil
}
