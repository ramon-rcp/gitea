// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/renderhelper"
	unit_model "gitea.dev/models/unit"
	"gitea.dev/modules/git"
	"gitea.dev/modules/markup/markdown"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/templates"
	"gitea.dev/modules/web"
	"gitea.dev/services/context"
	"gitea.dev/services/context/upload"
	"gitea.dev/services/forms"
	issue_service "gitea.dev/services/issue"
)

const (
	tplNewCommitComment    templates.TplName = "repo/commit/new_comment"
	tplCommitCommentThread templates.TplName = "repo/commit/comment_thread"
)

// RenderNewCommitCommentForm will render the form for creating a new commit comment
func RenderNewCommitCommentForm(ctx *context.Context) {
	ctx.Data["PageIsCommitFiles"] = true
	ctx.Data["CommitID"] = ctx.PathParam("sha")
	ctx.Data["IsAttachmentEnabled"] = setting.Attachment.Enabled
	upload.AddUploadContext(ctx, "commit_comment")
	ctx.HTML(http.StatusOK, tplNewCommitComment)
}

// CreateCommitComment creates a new comment on a commit's diff
func CreateCommitComment(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.CommitCommentForm)
	if ctx.HasError() {
		ctx.Flash.Error(ctx.GetErrMsg())
		ctx.Redirect(ctx.Repo.RepoLink + "/commit/" + ctx.PathParam("sha"))
		return
	}

	commitSHA := ctx.PathParam("sha")
	if _, err := ctx.Repo.GitRepo.GetCommit(commitSHA); err != nil {
		if git.IsErrNotExist(err) {
			ctx.NotFound(err)
		} else {
			ctx.ServerError("GetCommit", err)
		}
		return
	}

	signedLine := form.Line
	if form.Side == "previous" {
		signedLine *= -1
	}

	var attachments []string
	if setting.Attachment.Enabled {
		attachments = form.Files
	}

	comment, err := issue_service.CreateCommitComment(ctx, ctx.Doer, ctx.Repo.Repository, commitSHA, form.TreePath, signedLine, form.Content, attachments)
	if err != nil {
		ctx.ServerError("CreateCommitComment", err)
		return
	}

	renderCommitCommentThread(ctx, commitSHA, comment.TreePath, comment.Line)
}

// renderCommitCommentThread re-renders the full comment thread for one diff line, used
// as the response after creating, editing, or deleting a comment on that line.
func renderCommitCommentThread(ctx *context.Context, commitSHA, treePath string, line int64) {
	allComments, err := issues_model.FetchCommitCodeComments(ctx, ctx.Repo.Repository, commitSHA)
	if err != nil {
		ctx.ServerError("FetchCommitCodeComments", err)
		return
	}

	ctx.Data["CommitID"] = commitSHA
	ctx.Data["IsAttachmentEnabled"] = setting.Attachment.Enabled
	upload.AddUploadContext(ctx, "commit_comment")
	ctx.Data["comments"] = allComments[treePath][line]
	ctx.HTML(http.StatusOK, tplCommitCommentThread)
}

// UpdateCommitCommentContentRoute updates the content of a commit comment
func UpdateCommitCommentContentRoute(ctx *context.Context) {
	comment, err := issues_model.GetCommitCommentByID(ctx, ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("GetCommitCommentByID", issues_model.IsErrCommitCommentNotExist, err)
		return
	}

	if comment.RepoID != ctx.Repo.Repository.ID {
		ctx.NotFound(issues_model.ErrCommitCommentNotExist{})
		return
	}

	if ctx.Doer.ID != comment.PosterID && !ctx.Repo.Permission.CanWrite(unit_model.TypeCode) {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	newContent := ctx.FormString("content")
	contentVersion := ctx.FormInt("content_version")
	if contentVersion != comment.ContentVersion {
		ctx.JSONError(ctx.Tr("repo.comments.edit.already_changed"))
		return
	}

	if newContent != comment.Content {
		comment.Content = newContent
		if err := issues_model.UpdateCommitComment(ctx, comment, contentVersion); err != nil {
			if errors.Is(err, issues_model.ErrCommentAlreadyChanged) {
				ctx.JSONError(ctx.Tr("repo.comments.edit.already_changed"))
			} else {
				ctx.ServerError("UpdateCommitComment", err)
			}
			return
		}
	}

	var renderedContent template.HTML
	if comment.Content != "" {
		rctx := renderhelper.NewRenderContextRepoComment(ctx, ctx.Repo.Repository, renderhelper.RepoCommentOptions{
			FootnoteContextID: strconv.FormatInt(comment.ID, 10),
		})
		renderedContent, err = markdown.RenderString(rctx, comment.Content)
		if err != nil {
			ctx.ServerError("RenderString", err)
			return
		}
	}

	ctx.JSON(http.StatusOK, map[string]any{
		"content":        commentContentHTML(ctx, renderedContent),
		"contentVersion": comment.ContentVersion,
	})
}

// DeleteCommitCommentRoute deletes a commit comment
func DeleteCommitCommentRoute(ctx *context.Context) {
	comment, err := issues_model.GetCommitCommentByID(ctx, ctx.PathParamInt64("id"))
	if err != nil {
		ctx.NotFoundOrServerError("GetCommitCommentByID", issues_model.IsErrCommitCommentNotExist, err)
		return
	}

	if comment.RepoID != ctx.Repo.Repository.ID {
		ctx.NotFound(issues_model.ErrCommitCommentNotExist{})
		return
	}

	if ctx.Doer.ID != comment.PosterID && !ctx.Repo.Permission.CanWrite(unit_model.TypeCode) {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	if err := issues_model.DeleteCommitComment(ctx, comment); err != nil {
		ctx.ServerError("DeleteCommitComment", err)
		return
	}

	ctx.Status(http.StatusOK)
}
