// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/models/db"
	"gitea.dev/modules/timeutil"
)

func AddCommitCommentTable(x db.EngineMigration) error {
	type CommitComment struct {
		ID             int64              `xorm:"pk autoincr"`
		PosterID       int64              `xorm:"INDEX"`
		RepoID         int64              `xorm:"INDEX NOT NULL"`
		CommitSHA      string             `xorm:"INDEX(commit_line) VARCHAR(64) NOT NULL"`
		TreePath       string             `xorm:"VARCHAR(4000)"`
		Line           int64              `xorm:"INDEX(commit_line)"`
		Content        string             `xorm:"LONGTEXT"`
		ContentVersion int                `xorm:"NOT NULL DEFAULT 0"`
		CreatedUnix    timeutil.TimeStamp `xorm:"INDEX created"`
		UpdatedUnix    timeutil.TimeStamp `xorm:"INDEX updated"`
	}
	if err := x.Sync(new(CommitComment)); err != nil {
		return err
	}

	type Attachment struct {
		CommitCommentID int64 `xorm:"INDEX"`
	}
	return x.Sync(new(Attachment))
}
