// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"fmt"
	"html/template"
	"strconv"

	"gitea.dev/models/db"
	"gitea.dev/models/renderhelper"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/markup/markdown"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"
)

// CommitComment represents a comment attached directly to a commit, independent of
// any issue or pull request. Unlike Comment, it is not linked via IssueID: a commit
// SHA is immutable, so there is no "outdated" or review-pending state to track.
type CommitComment struct {
	ID              int64                    `xorm:"pk autoincr"`
	PosterID        int64                    `xorm:"INDEX"`
	Poster          *user_model.User         `xorm:"-"`
	RepoID          int64                    `xorm:"INDEX NOT NULL"`
	CommitSHA       string                   `xorm:"INDEX(commit_line) VARCHAR(64) NOT NULL"`
	TreePath        string                   `xorm:"VARCHAR(4000)"`      // SQLServer limit, mirrors Comment.TreePath
	Line            int64                    `xorm:"INDEX(commit_line)"` // same sign convention as Comment.Line: - = previous/left, + = proposed/right
	Content         string                   `xorm:"LONGTEXT"`
	ContentVersion  int                      `xorm:"NOT NULL DEFAULT 0"`
	RenderedContent template.HTML            `xorm:"-"`
	Attachments     []*repo_model.Attachment `xorm:"-"`
	CreatedUnix     timeutil.TimeStamp       `xorm:"INDEX created"`
	UpdatedUnix     timeutil.TimeStamp       `xorm:"INDEX updated"`
}

func init() {
	db.RegisterModel(new(CommitComment))
}

// ErrCommitCommentNotExist represents a "CommitCommentNotExist" kind of error.
type ErrCommitCommentNotExist struct {
	ID int64
}

// IsErrCommitCommentNotExist checks if an error is a ErrCommitCommentNotExist.
func IsErrCommitCommentNotExist(err error) bool {
	_, ok := err.(ErrCommitCommentNotExist)
	return ok
}

func (err ErrCommitCommentNotExist) Error() string {
	return fmt.Sprintf("commit comment does not exist [id: %d]", err.ID)
}

func (err ErrCommitCommentNotExist) Unwrap() error {
	return util.ErrNotExist
}

// CommitCommentHashTag returns unique hash tag for a commit comment. It uses a
// different prefix than CommentHashTag so the two can never collide as HTML anchors
// on the same page.
func CommitCommentHashTag(id int64) string {
	return fmt.Sprintf("commitcomment-%d", id)
}

// HashTag returns unique hash tag for the commit comment.
func (c *CommitComment) HashTag() string {
	return CommitCommentHashTag(c.ID)
}

// DiffSide returns "previous" if Line is a LOC of the previous file, otherwise "proposed"
func (c *CommitComment) DiffSide() string {
	if c.Line < 0 {
		return "previous"
	}
	return "proposed"
}

// UnsignedLine returns the LOC of the commit comment without + or -
func (c *CommitComment) UnsignedLine() uint64 {
	if c.Line < 0 {
		return uint64(c.Line * -1)
	}
	return uint64(c.Line)
}

// LoadPoster loads the commit comment poster
func (c *CommitComment) LoadPoster(ctx context.Context) (err error) {
	if c.Poster != nil {
		return nil
	}
	c.PosterID, c.Poster, err = user_model.GetPossibleUserByID(ctx, c.PosterID)
	return err
}

// LoadAttachments loads the attachments bound to this commit comment (never returns
// an error itself; a lookup failure just leaves Attachments empty).
func (c *CommitComment) LoadAttachments(ctx context.Context) error {
	if len(c.Attachments) > 0 {
		return nil
	}

	var err error
	c.Attachments, err = repo_model.GetAttachmentsByCommitCommentID(ctx, c.ID)
	return err
}

// CommitCommentList defines a list of commit comments
type CommitCommentList []*CommitComment

// LoadPosters loads the posters for a list of commit comments in one batch query
func (comments CommitCommentList) LoadPosters(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	posterIDs := container.FilterSlice(comments, func(c *CommitComment) (int64, bool) {
		return c.PosterID, c.Poster == nil && c.PosterID > 0
	})

	posterMaps, err := user_model.GetUsersMapByIDs(ctx, posterIDs)
	if err != nil {
		return err
	}

	for _, comment := range comments {
		if comment.Poster == nil {
			comment.Poster = user_model.GetPossibleUserFromMap(comment.PosterID, posterMaps)
		}
	}
	return nil
}

// LoadAttachments loads the attachments for a list of commit comments in one batch query
func (comments CommitCommentList) LoadAttachments(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	commentIDs := make([]int64, 0, len(comments))
	for _, c := range comments {
		commentIDs = append(commentIDs, c.ID)
	}

	attachments := make(map[int64][]*repo_model.Attachment, len(comments))
	rows, err := db.GetEngine(ctx).In("commit_comment_id", commentIDs).Rows(new(repo_model.Attachment))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var attachment repo_model.Attachment
		if err := rows.Scan(&attachment); err != nil {
			return err
		}
		attachments[attachment.CommitCommentID] = append(attachments[attachment.CommitCommentID], &attachment)
	}

	for _, comment := range comments {
		comment.Attachments = attachments[comment.ID]
	}
	return nil
}

// CreateCommitCommentOptions describes the input needed to create a CommitComment
type CreateCommitCommentOptions struct {
	Doer        *user_model.User
	Repo        *repo_model.Repository
	CommitSHA   string
	TreePath    string
	Line        int64
	Content     string
	Attachments []string // uuids of already-uploaded, unbound attachments
}

// CreateCommitComment creates a comment on a commit and binds any pending attachments to it.
func CreateCommitComment(ctx context.Context, opts *CreateCommitCommentOptions) (*CommitComment, error) {
	return db.WithTx2(ctx, func(ctx context.Context) (*CommitComment, error) {
		comment := &CommitComment{
			PosterID:  opts.Doer.ID,
			Poster:    opts.Doer,
			RepoID:    opts.Repo.ID,
			CommitSHA: opts.CommitSHA,
			TreePath:  opts.TreePath,
			Line:      opts.Line,
			Content:   opts.Content,
		}
		if err := db.Insert(ctx, comment); err != nil {
			return nil, err
		}

		if err := UpdateCommitCommentAttachments(ctx, comment, opts.Attachments); err != nil {
			return nil, err
		}

		return comment, nil
	})
}

// UpdateCommitCommentAttachments binds already-uploaded attachments (identified by UUID) to the commit comment.
func UpdateCommitCommentAttachments(ctx context.Context, c *CommitComment, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	return db.WithTx(ctx, func(ctx context.Context) error {
		attachments, err := repo_model.GetAttachmentsByUUIDs(ctx, uuids)
		if err != nil {
			return fmt.Errorf("getAttachmentsByUUIDs [uuids: %v]: %w", uuids, err)
		}
		for i := range attachments {
			attachments[i].CommitCommentID = c.ID
			if err := repo_model.UpdateAttachment(ctx, attachments[i]); err != nil {
				return fmt.Errorf("update attachment [id: %d]: %w", attachments[i].ID, err)
			}
		}
		c.Attachments = attachments
		return nil
	})
}

// GetCommitCommentByID returns a commit comment by ID
func GetCommitCommentByID(ctx context.Context, id int64) (*CommitComment, error) {
	c := new(CommitComment)
	has, err := db.GetEngine(ctx).ID(id).Get(c)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrCommitCommentNotExist{ID: id}
	}
	return c, nil
}

// DeleteCommitComment deletes a commit comment and any attachments bound to it.
func DeleteCommitComment(ctx context.Context, comment *CommitComment) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		if _, err := db.GetEngine(ctx).ID(comment.ID).Delete(new(CommitComment)); err != nil {
			return err
		}
		_, err := repo_model.DeleteAttachmentsByCommitComment(ctx, comment.ID, true)
		return err
	})
}

// CommitCodeComments represents comments on a commit's diff, keyed the same way as
// CodeComments: FILENAME -> LINE (+ == proposed; - == previous) -> COMMENTS
type CommitCodeComments map[string]map[int64][]*CommitComment

// FetchCommitCodeComments returns all comments on a commit, grouped by file path and line.
// Unlike FetchCodeComments, there is no review/pending-visibility or outdated-comment
// filtering to apply: a commit SHA never changes, so every comment on it is always current.
func FetchCommitCodeComments(ctx context.Context, repo *repo_model.Repository, commitSHA string) (CommitCodeComments, error) {
	var comments CommitCommentList
	if err := db.GetEngine(ctx).
		Where("repo_id = ? AND commit_sha = ?", repo.ID, commitSHA).
		Asc("created_unix").
		Asc("id").
		Find(&comments); err != nil {
		return nil, err
	}

	if err := comments.LoadPosters(ctx); err != nil {
		return nil, err
	}
	if err := comments.LoadAttachments(ctx); err != nil {
		return nil, err
	}

	pathToLineToComment := make(CommitCodeComments)
	for _, comment := range comments {
		rctx := renderhelper.NewRenderContextRepoComment(ctx, repo, renderhelper.RepoCommentOptions{
			FootnoteContextID: strconv.FormatInt(comment.ID, 10),
		})
		var err error
		if comment.RenderedContent, err = markdown.RenderString(rctx, comment.Content); err != nil {
			return nil, err
		}

		if pathToLineToComment[comment.TreePath] == nil {
			pathToLineToComment[comment.TreePath] = make(map[int64][]*CommitComment)
		}
		pathToLineToComment[comment.TreePath][comment.Line] = append(pathToLineToComment[comment.TreePath][comment.Line], comment)
	}
	return pathToLineToComment, nil
}
