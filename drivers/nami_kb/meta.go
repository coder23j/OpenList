package nami_kb

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	driver.RootID
	Cookie string `json:"cookie" type:"text" required:"true" help:"360 account cookie value"`
}

var config = driver.Config{
	Name:        "NamiKB",
	DefaultRoot: "/",
	NoUpload:    true,
	NoCache:     true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &NamiKB{}
	})
}
