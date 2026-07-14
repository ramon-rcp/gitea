// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issue

import (
	"testing"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCreateCommitComment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	// simulates an attachment already uploaded (via the /commit/{sha}/attachments
	// endpoint) before the comment referencing it is submitted
	attachment := repo_model.Attachment{Name: "screenshot.png", UUID: uuid.New().String()}
	assert.NoError(t, db.Insert(t.Context(), &attachment))

	comment, err := CreateCommitComment(t.Context(), doer, repo, "65f1bf27bc3bf70f64657658635e66094edbcb4", "README.md", 4, "see attached", []string{attachment.UUID})
	assert.NoError(t, err)

	assert.Equal(t, repo.ID, comment.RepoID)
	assert.Equal(t, doer.ID, comment.PosterID)
	assert.Equal(t, "65f1bf27bc3bf70f64657658635e66094edbcb4", comment.CommitSHA)
	assert.Equal(t, "README.md", comment.TreePath)
	assert.Equal(t, int64(4), comment.Line)
	unittest.AssertExistsAndLoadBean(t, &issues_model.CommitComment{ID: comment.ID})

	// the crux of the feature: the router-facing service call must translate through
	// to the same correct attachment binding as the model layer
	updated := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: attachment.ID})
	assert.Equal(t, comment.ID, updated.CommitCommentID)
}
