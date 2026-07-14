// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

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

func TestIncreaseDownloadCount(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	attachment, err := repo_model.GetAttachmentByUUID(t.Context(), "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), attachment.DownloadCount)

	// increase download count
	err = attachment.IncreaseDownloadCount(t.Context())
	assert.NoError(t, err)

	attachment, err = repo_model.GetAttachmentByUUID(t.Context(), "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), attachment.DownloadCount)
}

func TestGetByCommentOrIssueID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// count of attachments from issue ID
	attachments, err := repo_model.GetAttachmentsByIssueID(t.Context(), 1)
	assert.NoError(t, err)
	assert.Len(t, attachments, 1)

	attachments, err = repo_model.GetAttachmentsByCommentID(t.Context(), 1)
	assert.NoError(t, err)
	assert.Len(t, attachments, 2)
}

func TestDeleteAttachments(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	count, err := repo_model.DeleteAttachmentsByIssue(t.Context(), 4, false)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = repo_model.DeleteAttachmentsByComment(t.Context(), 2, false)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	err = repo_model.DeleteAttachment(t.Context(), &repo_model.Attachment{ID: 8}, false)
	assert.NoError(t, err)

	attachment, err := repo_model.GetAttachmentByUUID(t.Context(), "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a18")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrAttachmentNotExist(err))
	assert.Nil(t, attachment)
}

func TestGetAttachmentByID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	attach, err := repo_model.GetAttachmentByID(t.Context(), 1)
	assert.NoError(t, err)
	assert.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", attach.UUID)
}

func TestAttachment_DownloadURL(t *testing.T) {
	attach := &repo_model.Attachment{
		UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
		ID:   1,
	}
	assert.Equal(t, "https://try.gitea.io/attachments/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", attach.DownloadURL())
}

func TestUpdateAttachment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	attach, err := repo_model.GetAttachmentByID(t.Context(), 1)
	assert.NoError(t, err)
	assert.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", attach.UUID)

	attach.Name = "new_name"
	assert.NoError(t, repo_model.UpdateAttachment(t.Context(), attach))

	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{Name: "new_name"})
}

func TestGetAttachmentsByUUIDs(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	attachList, err := repo_model.GetAttachmentsByUUIDs(t.Context(), []string{"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a17", "not-existing-uuid"})
	assert.NoError(t, err)
	assert.Len(t, attachList, 2)
	assert.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", attachList[0].UUID)
	assert.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a17", attachList[1].UUID)
	assert.Equal(t, int64(1), attachList[0].IssueID)
	assert.Equal(t, int64(5), attachList[1].IssueID)
}

func TestGetUnlinkedAttachmentsByUserID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	attachments, err := repo_model.GetUnlinkedAttachmentsByUserID(t.Context(), 8)
	assert.NoError(t, err)
	assert.Len(t, attachments, 1)
	assert.Equal(t, int64(10), attachments[0].ID)
	assert.Zero(t, attachments[0].IssueID)
	assert.Zero(t, attachments[0].ReleaseID)
	assert.Zero(t, attachments[0].CommentID)

	attachments, err = repo_model.GetUnlinkedAttachmentsByUserID(t.Context(), 1)
	assert.NoError(t, err)
	assert.Empty(t, attachments)
}

func newTestCommitComment(t *testing.T) *issues_model.CommitComment {
	t.Helper()
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})
	comment, err := issues_model.CreateCommitComment(t.Context(), &issues_model.CreateCommitCommentOptions{
		Doer:      doer,
		Repo:      repo,
		CommitSHA: "65f1bf27bc3bf70f64657658635e66094edbcb4",
		TreePath:  "README.md",
		Line:      4,
		Content:   "x",
	})
	assert.NoError(t, err)
	return comment
}

func TestGetAndDeleteAttachmentsByCommitCommentID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	comment := newTestCommitComment(t)
	attach1 := repo_model.Attachment{Name: "a.png", UUID: uuid.New().String()}
	attach2 := repo_model.Attachment{Name: "b.png", UUID: uuid.New().String()}
	assert.NoError(t, db.Insert(t.Context(), &attach1))
	assert.NoError(t, db.Insert(t.Context(), &attach2))
	assert.NoError(t, issues_model.UpdateCommitCommentAttachments(t.Context(), comment, []string{attach1.UUID, attach2.UUID}))

	attachments, err := repo_model.GetAttachmentsByCommitCommentID(t.Context(), comment.ID)
	assert.NoError(t, err)
	assert.Len(t, attachments, 2)

	count, err := repo_model.DeleteAttachmentsByCommitComment(t.Context(), comment.ID, false)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	attachments, err = repo_model.GetAttachmentsByCommitCommentID(t.Context(), comment.ID)
	assert.NoError(t, err)
	assert.Empty(t, attachments)
}

// TestCountAndDeleteOrphanedAttachments_CommitComment is the single most important
// regression test for this feature: prior attempts at "comments on commits" reportedly
// failed because attachments bound only to a comment (no IssueID/ReleaseID) were either
// invisible to the orphan sweep, or downloadable only by their uploader. This proves the
// orphan sweep now recognizes CommitCommentID, while a still-valid one is left alone.
func TestCountAndDeleteOrphanedAttachments_CommitComment(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	comment := newTestCommitComment(t)

	valid := repo_model.Attachment{Name: "valid.png", UUID: uuid.New().String(), CommitCommentID: comment.ID}
	assert.NoError(t, db.Insert(t.Context(), &valid))

	// simulates the comment row having been removed by something other than the ORM's
	// own cascading delete (issues_model.DeleteCommitComment) -- e.g. a raw SQL cleanup
	orphaned := repo_model.Attachment{Name: "orphaned.png", UUID: uuid.New().String(), CommitCommentID: comment.ID + 999999}
	assert.NoError(t, db.Insert(t.Context(), &orphaned))

	before, err := repo_model.CountOrphanedAttachments(t.Context())
	assert.NoError(t, err)

	assert.NoError(t, repo_model.DeleteOrphanedAttachments(t.Context()))

	unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: valid.ID}) // untouched
	unittest.AssertNotExistsBean(t, &repo_model.Attachment{ID: orphaned.ID})  // swept

	after, err := repo_model.CountOrphanedAttachments(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, before-1, after)
}
