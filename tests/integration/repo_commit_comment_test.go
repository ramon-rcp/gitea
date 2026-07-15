// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"

	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func testCreateCommitCommentAttachment(t *testing.T, session *TestSession, repoURL, commitSHA, filename string, content []byte, expectedStatus int) string {
	body := &bytes.Buffer{}

	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	assert.NoError(t, err)
	_, err = part.Write(content)
	assert.NoError(t, err)
	assert.NoError(t, writer.Close())

	req := NewRequestWithBody(t, "POST", repoURL+"/commit/"+commitSHA+"/attachments", body)
	req.Header.Add("Content-Type", writer.FormDataContentType())
	resp := session.MakeRequest(t, req, expectedStatus)

	if expectedStatus != http.StatusOK {
		return ""
	}
	obj := DecodeJSON(t, resp, map[string]string{})
	return obj["uuid"]
}

// TestRepoCommitComment is the full HTTP round trip for the "comments on commits"
// feature: render the plain commit page (no PR involved), POST a new inline comment
// with an attachment, re-render, and confirm both the comment and the attachment are
// visible. The attachment-visible-to-a-different-user assertion is the exact
// regression the issue is about: attachments bound only to a comment (no IssueID)
// used to resolve to unit.TypeInvalid and be downloadable only by their uploader.
func TestRepoCommitComment(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const repoURL = "/user2/repo1"
	const commitSHA = "65f1bf27bc3bf70f64657658635e66094edbcb4d"

	owner := loginUser(t, "user2") // repo1's owner, creates the comment
	other := loginUser(t, "user4") // an unrelated signed-in user, must still be able to view + download

	uuid := testCreateCommitCommentAttachment(t, owner, repoURL, commitSHA, "note.png", testGeneratePngBytes(), http.StatusOK)

	// commitSHA is repo1's initial commit, which only adds 3 lines to README.md, so
	// line 1 (proposed/right side) is the only guaranteed-valid coordinate to comment on
	req := NewRequestWithValues(t, "POST", repoURL+"/commit/"+commitSHA+"/comments", map[string]string{
		"content": "why is this line here?",
		"side":    "proposed",
		"line":    "1",
		"path":    "README.md",
		"files":   uuid,
	})
	owner.MakeRequest(t, req, http.StatusOK)

	req = NewRequest(t, "GET", repoURL+"/commit/"+commitSHA)
	resp := owner.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)
	assert.Contains(t, htmlDoc.doc.Find(".comment .render-content").Text(), "why is this line here?")
	assert.Equal(t, 1, htmlDoc.doc.Find(".dropzone-attachments").Length())

	other.MakeRequest(t, NewRequest(t, "GET", "/attachments/"+uuid), http.StatusOK)
}

// TestPRFilesChangedNewCommentURLUnaffected guards box.tmpl's data-new-comment-url
// against regressing for PRs: adding the PageIsCommitFiles branch alongside the
// existing PageIsPullFiles one must not change what a PR's "Files changed" tab points
// at. There was no prior integration coverage of this attribute at all.
func TestPRFilesChangedNewCommentURLUnaffected(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")
	req := NewRequest(t, "GET", "/user2/repo1/pulls/3/files")
	resp := session.MakeRequest(t, req, http.StatusOK)

	htmlDoc := NewHTMLParser(t, resp.Body)
	url, exists := htmlDoc.doc.Find("table.chroma").First().Attr("data-new-comment-url")
	assert.True(t, exists)
	assert.Equal(t, "/user2/repo1/pulls/3/files/reviews/new_comment", url)
}
