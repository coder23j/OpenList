package nami_kb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/go-resty/resty/v2"
)

const (
	kbAPIBase       = "https://kb-api.n.cn"
	downloadAPIBase = "https://openapi.eyun.360.cn"
	idSeparator     = "|"
)

type NamiKB struct {
	model.Storage
	Addition
	client *resty.Client
}

func (d *NamiKB) Config() driver.Config        { return config }
func (d *NamiKB) GetAddition() driver.Additional { return &d.Addition }

func (d *NamiKB) Init(ctx context.Context) error {
	d.Cookie = strings.TrimSpace(d.Cookie)
	if d.Cookie == "" {
		return errors.New("cookie is required")
	}
	d.client = resty.New().
		SetBaseURL(kbAPIBase).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Content-Type", "application/json").
		SetHeader("Cookie", d.Cookie).
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	return nil
}

func (d *NamiKB) Drop(ctx context.Context) error {
	return nil
}

func (d *NamiKB) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	parentID := dir.GetID()
	if parentID == d.RootFolderID {
		return d.listKnowledgeBases(ctx)
	}
	return d.listFiles(ctx, parentID)
}

func (d *NamiKB) listKnowledgeBases(ctx context.Context) ([]model.Obj, error) {
	var resp folderListResp
	r, err := d.client.R().
		SetContext(ctx).
		SetBody(map[string]any{"page": 1, "size": 50, "type": 0}).
		SetResult(&resp).
		Post("/main/v1/knowledge/datasets/list")
	if err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}
	if r.IsError() {
		return nil, fmt.Errorf("list knowledge bases: HTTP %d", r.StatusCode())
	}
	if resp.Errno != 0 {
		return nil, fmt.Errorf("list knowledge bases: %s", resp.Errmsg)
	}

	res := make([]model.Obj, 0, len(resp.Data.List))
	for _, item := range resp.Data.List {
		name := strings.Trim(item.Name, "/")
		if name == "" {
			continue
		}
		res = append(res, &model.Object{
			ID:       item.FolderID,
			Name:     name,
			IsFolder: true,
		})
	}
	return res, nil
}

func (d *NamiKB) listFiles(ctx context.Context, folderID string) ([]model.Obj, error) {
	const pageSize = 50
	res := make([]model.Obj, 0)
	for page := 1; ; page++ {
		var resp fileListResp
		r, err := d.client.R().
			SetContext(ctx).
			SetBody(map[string]any{
				"folder_id":  folderID,
				"page":       page,
				"ks_id":      "",
				"sort_type":  2,
				"order_type": 2,
				"size":       pageSize,
			}).
			SetResult(&resp).
			Post("/main/v1/knowledge/file/list")
		if err != nil {
			return nil, fmt.Errorf("list files: %w", err)
		}
		if r.IsError() {
			return nil, fmt.Errorf("list files: HTTP %d", r.StatusCode())
		}
		if resp.Errno != 0 {
			return nil, fmt.Errorf("list files: %s", resp.Errmsg)
		}

		for _, item := range resp.Data.List {
			if item.Name == "" {
				continue
			}
			res = append(res, &model.Object{
				ID:       encodeFileID(item.FileID, item.ShareQid),
				Name:     item.Name,
				Size:     item.SizeInt(),
				Modified: unixOrZero(item.ModifyTimeInt()),
				Ctime:    unixOrZero(item.CreateTimeInt()),
			})
		}

		if len(resp.Data.List) < pageSize || len(res) >= resp.Data.Total {
			break
		}
	}
	return res, nil
}

func (d *NamiKB) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	fileID, qid := decodeFileID(file.GetID())
	if fileID == "" || qid == "" {
		return nil, errors.New("invalid file id, cannot extract file_id or qid")
	}

	accessToken, err := d.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var resp downloadResp
	r, err := resty.New().R().
		SetContext(ctx).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Access-Token", accessToken).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]any{
			"nid":        fileID,
			"disposition": "attachment",
			"file_ext":   `{"ks_id":""}`,
		}).
		SetResult(&resp).
		Post(fmt.Sprintf("%s/intf.php?qid=%s&method=Sync.getDownLoadUrl&allowed_channel=360NaMi&sub_channel=360NaMi_WEB",
			downloadAPIBase, qid))
	if err != nil {
		return nil, fmt.Errorf("get download url: %w", err)
	}
	if r.IsError() {
		return nil, fmt.Errorf("get download url: HTTP %d", r.StatusCode())
	}
	if resp.Errno != 0 {
		return nil, fmt.Errorf("get download url: %s", resp.Errmsg)
	}

	url := strings.TrimSpace(resp.Data.DownloadURL)
	if url == "" {
		return nil, errors.New("empty download url")
	}
	return &model.Link{URL: url}, nil
}

func (d *NamiKB) getAccessToken(ctx context.Context) (string, error) {
	var resp tokenResp
	r, err := d.client.R().
		SetContext(ctx).
		SetResult(&resp).
		Post("/main/v1/knowledge/cloud/token")
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}
	if r.IsError() {
		return "", fmt.Errorf("get token: HTTP %d", r.StatusCode())
	}
	if resp.Errno != 0 {
		return "", fmt.Errorf("get token: %s", resp.Errmsg)
	}
	token := strings.TrimSpace(resp.Data.AccessToken)
	if token == "" {
		return "", errors.New("empty access_token")
	}
	return token, nil
}

// encodeFileID combines file_id and qid into a single ID string.
func encodeFileID(fileID, qid string) string {
	return fileID + idSeparator + qid
}

// decodeFileID splits a combined ID back into file_id and qid.
func decodeFileID(id string) (fileID, qid string) {
	parts := strings.SplitN(id, idSeparator, 2)
	if len(parts) != 2 {
		return id, ""
	}
	return parts[0], parts[1]
}

var _ driver.Driver = (*NamiKB)(nil)
