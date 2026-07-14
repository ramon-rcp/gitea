// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issue

import (
	"context"

	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
)

// CreateCommitComment creates a comment on a commit's diff, binding any pending
// attachments (identified by UUID) to it.
//
// Deletion and content updates have no logic beyond what issues_model.DeleteCommitComment
// and issues_model.UpdateCommitComment already do, so callers use those directly instead
// of a pass-through wrapper here (see routers/web/repo/commit_comment.go).
func CreateCommitComment(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, commitSHA, treePath string, line int64, content string, attachmentUUIDs []string) (*issues_model.CommitComment, error) {
	return issues_model.CreateCommitComment(ctx, &issues_model.CreateCommitCommentOptions{
		Doer:        doer,
		Repo:        repo,
		CommitSHA:   commitSHA,
		TreePath:    treePath,
		Line:        line,
		Content:     content,
		Attachments: attachmentUUIDs,
	})
}
