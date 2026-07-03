package atomgit

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	driver.RootPath
	Endpoint       string `json:"endpoint" type:"string" help:"AtomGit API endpoint, default https://api.atomgit.com/api/v5"`
	Token          string `json:"token" type:"string" help:"AtomGit personal access token"`
	TokenType      string `json:"token_type" type:"select" options:"PRIVATE-TOKEN,Authorization,access_token" default:"PRIVATE-TOKEN" help:"Authentication method for token"`
	Owner          string `json:"owner" type:"string" required:"true" help:"Repository owner (user or organization)"`
	Repo           string `json:"repo" type:"string" required:"true" help:"Repository name"`
	Ref            string `json:"ref" type:"string" help:"Branch, tag or commit SHA, defaults to repository default branch"`
	DownloadProxy  string `json:"download_proxy" type:"string" help:"Prefix added before download URLs, e.g. https://mirror.example.com/"`
	Cookie         string `json:"cookie" type:"string" help:"Cookie for authentication (alternative to token)"`
	CommitterName  string `json:"committer_name" type:"string" help:"Committer name for write operations"`
	CommitterEmail string `json:"committer_email" type:"string" help:"Committer email for write operations"`
	AuthorName     string `json:"author_name" type:"string" help:"Author name for write operations"`
	AuthorEmail    string `json:"author_email" type:"string" help:"Author email for write operations"`
}

var config = driver.Config{
	Name:        "AtomGit",
	LocalSort:   true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &AtomGit{}
	})
}
