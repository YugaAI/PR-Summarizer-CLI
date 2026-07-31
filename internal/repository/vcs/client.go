package vcs

import "context"

type VCSClient interface {
	PostOrUpdateComment(ctx context.Context, prNumber int, body string) error
}
