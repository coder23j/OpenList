package gitee

import (
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

// Content represents a file or directory entry from the Gitee contents API.
type Content struct {
	Type        string `json:"type"` // "file" or "dir"
	Size        *int64 `json:"size"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Sha         string `json:"sha"`
	URL         string `json:"url"`
	HtmlURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
	Links       Links  `json:"_links"`
}

func (c Content) toModelObj() model.Obj {
	size := int64(0)
	if c.Size != nil {
		size = *c.Size
	}
	return &Object{
		Object: model.Object{
			ID:       c.Path,
			Path:     utils.FixAndCleanPath(c.Path),
			Name:     c.Name,
			Size:     size,
			Modified: time.Unix(0, 0),
			IsFolder: c.Type == "dir",
		},
		Sha:         c.Sha,
		DownloadURL: c.DownloadURL,
		HtmlURL:     c.HtmlURL,
	}
}

type Links struct {
	Self string `json:"self"`
	Html string `json:"html"`
}

// Object is the model object with Gitee-specific extra fields.
type Object struct {
	model.Object
	Sha         string
	DownloadURL string
	HtmlURL     string
}

func (o *Object) URL() string {
	return o.DownloadURL
}

// Repo holds the minimal repo info we need.
type Repo struct {
	DefaultBranch string `json:"default_branch"`
}

// ErrResp represents a Gitee API error response.
type ErrResp struct {
	Message string `json:"message"`
}
