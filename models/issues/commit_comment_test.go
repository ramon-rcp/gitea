// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues_test

import (
	"testing"
	"time"

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

	now := time.Now().Unix()
	comment, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer:      doer,
		Repo:      repo,
		CommitSHA: "65f1bf27bc3bf70f64657658635e66094edbcb4",
		TreePath:  "README.md",
		Line:      4,
		Content:   "why is this line here?",
	})
	assert.NoError(t, err)
	then := time.Now().Unix()

	assert.Equal(t, repo.ID, comment.RepoID)
	assert.Equal(t, doer.ID, comment.PosterID)
	assert.Equal(t, "65f1bf27bc3bf70f64657658635e66094edbcb4", comment.CommitSHA)
	assert.Equal(t, "README.md", comment.TreePath)
	assert.Equal(t, int64(4), comment.Line)
	assert.Equal(t, "why is this line here?", comment.Content)
	unittest.AssertInt64InRange(t, now, then, int64(comment.CreatedUnix))
	unittest.AssertExistsAndLoadBean(t, comment) // assert actually added to DB
}

func TestCreateCommitComment_AttachmentBinding(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	// an attachment that was uploaded but not yet bound to anything, matching how a
	// real upload works before the comment it belongs to has been created
	attachment := repo_model.Attachment{Name: "screenshot.png", UUID: uuid.New().String()}
	assert.NoError(t, db.Insert(t.Context(), &attachment))

	comment, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer:        doer,
		Repo:        repo,
		CommitSHA:   "65f1bf27bc3bf70f64657658635e66094edbcb4",
		TreePath:    "README.md",
		Line:        4,
		Content:     "see attached",
		Attachments: []string{attachment.UUID},
	})
	assert.NoError(t, err)

	// this is the exact bug the feature exists to fix: the attachment must end up
	// bound via CommitCommentID (not left with every FK at zero), so it is both
	// findable by GetAttachmentsByCommitCommentID and correctly resolvable by
	// GetAttachmentLinkedTypeAndRepoID / caught by the orphan sweep if ever dangling
	updated := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: attachment.ID})
	assert.Equal(t, comment.ID, updated.CommitCommentID)
	assert.Zero(t, updated.IssueID)
	assert.Zero(t, updated.ReleaseID)
	assert.Zero(t, updated.CommentID)

	fromComment, err := repo_model.GetAttachmentsByCommitCommentID(t.Context(), comment.ID)
	assert.NoError(t, err)
	assert.Len(t, fromComment, 1)
	assert.Equal(t, attachment.ID, fromComment[0].ID)
}

func TestFetchCommitCodeComments(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})
	const commitSHA = "65f1bf27bc3bf70f64657658635e66094edbcb4"

	readme4, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer: doer, Repo: repo, CommitSHA: commitSHA, TreePath: "README.md", Line: 4, Content: "first",
	})
	assert.NoError(t, err)
	readme4Reply, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer: doer, Repo: repo, CommitSHA: commitSHA, TreePath: "README.md", Line: 4, Content: "second, same line",
	})
	assert.NoError(t, err)
	mainGo1, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer: doer, Repo: repo, CommitSHA: commitSHA, TreePath: "main.go", Line: 1, Content: "different file",
	})
	assert.NoError(t, err)
	// a comment on a different commit must never show up when fetching commitSHA's comments
	_, err = issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer: doer, Repo: repo, CommitSHA: "057c8f3809ee0af3c1b46a1c522f6320c1082f14", TreePath: "README.md", Line: 4, Content: "other commit",
	})
	assert.NoError(t, err)

	grouped, err := issues_model.FetchCommitCodeComments(t.Context(), repo, commitSHA)
	assert.NoError(t, err)

	assert.ElementsMatch(t, []int64{readme4.ID, readme4Reply.ID}, []int64{grouped["README.md"][4][0].ID, grouped["README.md"][4][1].ID})
	assert.Len(t, grouped["main.go"][1], 1)
	assert.Equal(t, mainGo1.ID, grouped["main.go"][1][0].ID)
	assert.NotEmpty(t, grouped["README.md"][4][0].RenderedContent) // markdown should have been rendered
}

func TestDeleteCommitComment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	attachment := repo_model.Attachment{Name: "screenshot.png", UUID: uuid.New().String()}
	assert.NoError(t, db.Insert(t.Context(), &attachment))

	comment, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer:        doer,
		Repo:        repo,
		CommitSHA:   "65f1bf27bc3bf70f64657658635e66094edbcb4",
		TreePath:    "README.md",
		Line:        4,
		Content:     "temporary",
		Attachments: []string{attachment.UUID},
	})
	assert.NoError(t, err)

	assert.NoError(t, issues_model.DeleteCommitComment(t.Context(), comment))

	unittest.AssertNotExistsBean(t, &issues_model.CommitComment{ID: comment.ID})
	// deleting the comment must take its attachment with it, otherwise the attachment
	// is left dangling with CommitCommentID pointing at a row that no longer exists
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: attachment.ID})
}

func TestUpdateCommitComment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	comment, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer: doer, Repo: repo, CommitSHA: "65f1bf27bc3bf70f64657658635e66094edbcb4", TreePath: "README.md", Line: 4, Content: "original",
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, comment.ContentVersion)

	comment.Content = "edited"
	assert.NoError(t, issues_model.UpdateCommitComment(t.Context(), comment, 0))
	assert.Equal(t, 1, comment.ContentVersion)
	updated := unittest.AssertExistsAndLoadBean(t, &issues_model.CommitComment{ID: comment.ID})
	assert.Equal(t, "edited", updated.Content)

	// submitting against a stale content_version must be rejected, not silently overwrite
	comment.Content = "conflicting edit"
	err = issues_model.UpdateCommitComment(t.Context(), comment, 0)
	assert.ErrorIs(t, err, issues_model.ErrCommentAlreadyChanged)
}

func TestCommitComment_DiffSideAndUnsignedLine(t *testing.T) {
	previous := &issues_model.CommitComment{Line: -4}
	assert.Equal(t, "previous", previous.DiffSide())
	assert.Equal(t, uint64(4), previous.UnsignedLine())

	proposed := &issues_model.CommitComment{Line: 4}
	assert.Equal(t, "proposed", proposed.DiffSide())
	assert.Equal(t, uint64(4), proposed.UnsignedLine())
}

func TestCommitComment_HashTag(t *testing.T) {
	comment := &issues_model.CommitComment{ID: 42}
	assert.Equal(t, "commitcomment-42", comment.HashTag())
	// must never collide with Comment's own "issuecomment-*" anchors on the same page
	assert.NotEqual(t, issues_model.CommentHashTag(42), comment.HashTag())
}
