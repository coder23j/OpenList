package atomgit

import (
	"fmt"
	"net/url"

	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/go-resty/resty/v2"
)

// getRepo fetches the repository info to obtain the default branch.
func (d *AtomGit) getRepo() (*Repo, error) {
	req := d.client.R()
	d.addAuth(req)
	if d.Cookie != "" {
		req.SetHeader("Cookie", d.Cookie)
	}
	escapedOwner := url.PathEscape(d.Owner)
	escapedRepo := url.PathEscape(d.Repo)
	res, err := req.Get(fmt.Sprintf("/repos/%s/%s", escapedOwner, escapedRepo))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	var repo Repo
	if err := utils.Json.Unmarshal(res.Body(), &repo); err != nil {
		return nil, err
	}
	if repo.DefaultBranch == "" {
		return nil, fmt.Errorf("failed to fetch default branch")
	}
	return &repo, nil
}

// toErr converts a resty error response to a Go error.
func toErr(res *resty.Response) error {
	var errMsg ErrResp
	if err := utils.Json.Unmarshal(res.Body(), &errMsg); err == nil {
		if errMsg.Message != "" {
			return fmt.Errorf("%s: %s", res.Status(), errMsg.Message)
		}
		if errMsg.ErrorMessage != "" {
			return fmt.Errorf("%s: %s", res.Status(), errMsg.ErrorMessage)
		}
	}
	return fmt.Errorf("%s", res.Status())
}
