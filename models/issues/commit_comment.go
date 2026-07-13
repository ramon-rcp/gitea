type CommitComment struct {
    ID               int64 `xorm:"pk autoincr"`
    PosterID         int64 `xorm:"INDEX"`
    Poster           *user_model.User `xorm:"-"`
    RepoID           int64  `xorm:"INDEX NOT NULL"`
    CommitSHA        string `xorm:"INDEX(commit_line) VARCHAR(64) NOT NULL"`
    TreePath         string `xorm:"VARCHAR(4000)"` // SQLServer limit, mirrors Comment.TreePath
    Line             int64  `xorm:"INDEX(commit_line)"` // same sign convention as Comment.Line: - = previous/left, + = proposed/right
    Content          string `xorm:"LONGTEXT"`
    ContentVersion   int    `xorm:"NOT NULL DEFAULT 0"`
    RenderedContent  template.HTML `xorm:"-"`
    Attachments      []*repo_model.Attachment `xorm:"-"`
    CreatedUnix      timeutil.TimeStamp `xorm:"INDEX created"`
    UpdatedUnix      timeutil.TimeStamp `xorm:"INDEX updated"`
}