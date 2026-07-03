package atomgit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	stdpath "path"
	"strings"
	"sync"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type AtomGit struct {
	model.Storage
	Addition
	client  *resty.Client
	writeMu sync.Mutex
}

func (d *AtomGit) Config() driver.Config {
	return config
}

func (d *AtomGit) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *AtomGit) Init(ctx context.Context) error {
	d.RootFolderPath = utils.FixAndCleanPath(d.RootFolderPath)
	d.Endpoint = strings.TrimSpace(d.Endpoint)
	if d.Endpoint == "" {
		d.Endpoint = "https://api.atomgit.com/api/v5"
	}
	d.Endpoint = strings.TrimSuffix(d.Endpoint, "/")
	d.Owner = strings.TrimSpace(d.Owner)
	d.Repo = strings.TrimSpace(d.Repo)
	d.Token = strings.TrimSpace(d.Token)
	d.TokenType = strings.TrimSpace(d.TokenType)
	if d.TokenType == "" {
		d.TokenType = "PRIVATE-TOKEN"
	}
	d.Cookie = strings.TrimSpace(d.Cookie)
	d.DownloadProxy = strings.TrimSpace(d.DownloadProxy)
	if d.Owner == "" || d.Repo == "" {
		return errors.New("owner and repo are required")
	}
	d.client = base.NewRestyClient().
		SetBaseURL(d.Endpoint).
		SetHeader("Accept", "application/json")
	if d.Cookie != "" {
		d.client.SetHeader("Cookie", d.Cookie)
	}
	repo, err := d.getRepo()
	if err != nil {
		return err
	}
	d.Ref = strings.TrimSpace(d.Ref)
	if d.Ref == "" {
		d.Ref = repo.DefaultBranch
	}
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *AtomGit) Drop(ctx context.Context) error {
	return nil
}

// ==================== Read Operations ====================

func (d *AtomGit) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	relPath := d.relativePath(dir.GetPath())
	contents, err := d.listContents(relPath)
	if err != nil {
		return nil, err
	}
	objs := make([]model.Obj, 0, len(contents))
	for i := range contents {
		objs = append(objs, contents[i].toModelObj())
	}
	return objs, nil
}

func (d *AtomGit) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	relPath := d.relativePath(file.GetPath())
	content, err := d.getContent(relPath)
	if err != nil {
		return nil, err
	}
	if content.DownloadURL == "" {
		return nil, errors.New("empty download_url from AtomGit API")
	}
	dlURL := d.applyProxy(content.DownloadURL)
	return &model.Link{
		URL:    dlURL,
		Header: d.linkHeader(),
	}, nil
}

// ==================== Write Operations ====================

func (d *AtomGit) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	dirPath := joinPath(d.relativePath(parentDir.GetPath()), dirName)
	openlistPath := joinPath(dirPath, ".openlist")
	body := map[string]string{
		"content": base64.StdEncoding.EncodeToString([]byte("空文件")),
		"message": fmt.Sprintf("OpenList: mkdir %s", dirPath),
		"branch":  d.Ref,
	}
	d.addCommitterAndAuthor(body)
	escapedPath := encodePath(openlistPath)
	res, err := d.newRequest().SetBody(body).
		Post(fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(d.Owner), url.PathEscape(d.Repo), escapedPath))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	return &Object{
		Object: model.Object{
			Name:     dirName,
			Path:     utils.FixAndCleanPath(stdpath.Join(parentDir.GetPath(), dirName)),
			IsFolder: true,
		},
	}, nil
}

func (d *AtomGit) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	dstPath := joinPath(d.relativePath(dstDir.GetPath()), file.GetName())

	body := map[string]string{
		"content": encoded,
		"message": fmt.Sprintf("OpenList: upload %s", dstPath),
		"branch":  d.Ref,
	}
	d.addCommitterAndAuthor(body)

	escapedPath := encodePath(dstPath)
	apiURL := fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(d.Owner), url.PathEscape(d.Repo), escapedPath)

	existing, getContentErr := d.getContent(d.relativePath(stdpath.Join(dstDir.GetPath(), file.GetName())))
	if getContentErr == nil && existing != nil {
		body["sha"] = existing.Sha
		body["message"] = fmt.Sprintf("OpenList: update %s", dstPath)
		res, err := d.newRequest().SetBody(body).Put(apiURL)
		if err != nil {
			return nil, err
		}
		if res.IsError() {
			return nil, toErr(res)
		}
		up(100)
		return &Object{
			Object: model.Object{
				Name:     file.GetName(),
				Path:     utils.FixAndCleanPath(stdpath.Join(dstDir.GetPath(), file.GetName())),
				Size:     int64(len(data)),
				Modified: file.ModTime(),
			},
		}, nil
	}

	res, err := d.newRequest().SetBody(body).Post(apiURL)
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	up(100)
	return &Object{
		Object: model.Object{
			Name:     file.GetName(),
			Path:     utils.FixAndCleanPath(stdpath.Join(dstDir.GetPath(), file.GetName())),
			Size:     int64(len(data)),
			Modified: file.ModTime(),
		},
	}, nil
}

func (d *AtomGit) Remove(ctx context.Context, obj model.Obj) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	if obj.IsDir() {
		return d.removeDir(d.relativePath(obj.GetPath()))
	}
	return d.removeFile(d.relativePath(obj.GetPath()))
}

func (d *AtomGit) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	srcRelPath := d.relativePath(srcObj.GetPath())
	dstDirRelPath := d.relativePath(stdpath.Dir(srcObj.GetPath()))
	dstRelPath := joinPath(dstDirRelPath, newName)

	err := d.moveFile(srcRelPath, dstRelPath, fmt.Sprintf("OpenList: rename %s to %s", srcRelPath, newName))
	if err != nil {
		return nil, err
	}
	return &Object{
		Object: model.Object{
			Name:     newName,
			Path:     utils.FixAndCleanPath(stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName)),
			Size:     srcObj.GetSize(),
			Modified: srcObj.ModTime(),
			IsFolder: srcObj.IsDir(),
		},
	}, nil
}

func (d *AtomGit) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	srcRelPath := d.relativePath(srcObj.GetPath())
	dstDirRelPath := d.relativePath(dstDir.GetPath())
	dstRelPath := joinPath(dstDirRelPath, srcObj.GetName())

	if srcObj.IsDir() {
		if err := d.moveDir(srcRelPath, dstRelPath); err != nil {
			return nil, err
		}
	} else {
		if err := d.moveFile(srcRelPath, dstRelPath, fmt.Sprintf("OpenList: move %s to %s", srcRelPath, dstRelPath)); err != nil {
			return nil, err
		}
	}
	return &Object{
		Object: model.Object{
			Name:     srcObj.GetName(),
			Path:     utils.FixAndCleanPath(stdpath.Join(dstDir.GetPath(), srcObj.GetName())),
			Size:     srcObj.GetSize(),
			Modified: srcObj.ModTime(),
			IsFolder: srcObj.IsDir(),
		},
	}, nil
}

func (d *AtomGit) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	if srcObj.IsDir() {
		return nil, errors.New("copy directory is not supported by AtomGit API")
	}
	relPath := d.relativePath(srcObj.GetPath())
	content, err := d.getContent(relPath)
	if err != nil {
		return nil, err
	}
	dlURL := content.DownloadURL
	if dlURL == "" {
		return nil, errors.New("empty download_url, cannot copy")
	}
	req := d.client.R()
	d.addAuth(req)
	if d.Cookie != "" {
		req.SetHeader("Cookie", d.Cookie)
	}
	resp, err := req.Get(dlURL)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("download source file failed: %s", resp.Status())
	}
	dstRelPath := joinPath(d.relativePath(dstDir.GetPath()), srcObj.GetName())
	encoded := base64.StdEncoding.EncodeToString(resp.Body())
	body := map[string]string{
		"content": encoded,
		"message": fmt.Sprintf("OpenList: copy %s to %s", relPath, dstRelPath),
		"branch":  d.Ref,
	}
	d.addCommitterAndAuthor(body)
	escapedPath := encodePath(dstRelPath)
	res, err := d.newRequest().SetBody(body).
		Post(fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(d.Owner), url.PathEscape(d.Repo), escapedPath))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	return &Object{
		Object: model.Object{
			Name:     srcObj.GetName(),
			Path:     utils.FixAndCleanPath(stdpath.Join(dstDir.GetPath(), srcObj.GetName())),
			Size:     srcObj.GetSize(),
			Modified: srcObj.ModTime(),
		},
	}, nil
}

// ==================== Internal Helpers ====================

func (d *AtomGit) newRequest() *resty.Request {
	req := d.client.R()
	d.addAuth(req)
	if d.Ref != "" {
		req.SetQueryParam("ref", d.Ref)
	}
	return req
}

func (d *AtomGit) addAuth(req *resty.Request) {
	if d.Token == "" {
		return
	}
	switch d.TokenType {
	case "Authorization":
		req.SetHeader("Authorization", "Bearer "+d.Token)
	case "access_token":
		req.SetQueryParam("access_token", d.Token)
	default:
		req.SetHeader("PRIVATE-TOKEN", d.Token)
	}
}

func (d *AtomGit) linkHeader() http.Header {
	header := http.Header{}
	if d.Cookie != "" {
		header.Set("Cookie", d.Cookie)
	}
	if d.Token != "" {
		switch d.TokenType {
		case "Authorization":
			header.Set("Authorization", "Bearer "+d.Token)
		case "PRIVATE-TOKEN", "":
			header.Set("PRIVATE-TOKEN", d.Token)
		}
	}
	if len(header) == 0 {
		return nil
	}
	return header
}

func (d *AtomGit) apiPath(path string) string {
	escapedOwner := url.PathEscape(d.Owner)
	escapedRepo := url.PathEscape(d.Repo)
	if path == "" {
		return fmt.Sprintf("/repos/%s/%s/contents", escapedOwner, escapedRepo)
	}
	return fmt.Sprintf("/repos/%s/%s/contents/%s", escapedOwner, escapedRepo, encodePath(path))
}

func (d *AtomGit) listContents(path string) ([]Content, error) {
	res, err := d.newRequest().Get(d.apiPath(path))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	var contents []Content
	if err := utils.Json.Unmarshal(res.Body(), &contents); err != nil {
		return nil, err
	}
	for i := range contents {
		contents[i].Path = joinPath(path, contents[i].Name)
	}
	return contents, nil
}

func (d *AtomGit) getContent(path string) (*Content, error) {
	res, err := d.newRequest().Get(d.apiPath(path))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	var content Content
	if err := utils.Json.Unmarshal(res.Body(), &content); err != nil {
		return nil, err
	}
	if content.Type == "" {
		return nil, errors.New("invalid response")
	}
	if content.Path == "" {
		content.Path = path
	}
	return &content, nil
}

func (d *AtomGit) removeFile(relPath string) error {
	content, err := d.getContent(relPath)
	if err != nil {
		return err
	}
	body := map[string]string{
		"sha":     content.Sha,
		"message": fmt.Sprintf("OpenList: delete %s", relPath),
		"branch":  d.Ref,
	}
	d.addCommitterAndAuthor(body)
	escapedPath := encodePath(relPath)
	res, err := d.newRequest().SetBody(body).
		Delete(fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(d.Owner), url.PathEscape(d.Repo), escapedPath))
	if err != nil {
		return err
	}
	if res.IsError() {
		return toErr(res)
	}
	return nil
}

func (d *AtomGit) removeDir(relPath string) error {
	contents, err := d.listContents(relPath)
	if err != nil {
		return err
	}
	for _, c := range contents {
		childRelPath := joinPath(relPath, c.Name)
		if c.Type == "dir" {
			if err := d.removeDir(childRelPath); err != nil {
				return err
			}
		} else {
			if err := d.removeFile(childRelPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *AtomGit) moveFile(srcRelPath, dstRelPath, message string) error {
	body := map[string]string{
		"from":    srcRelPath,
		"to":      dstRelPath,
		"message": message,
		"branch":  d.Ref,
	}
	d.addCommitterAndAuthor(body)
	escapedPath := encodePath(srcRelPath)
	res, err := d.newRequest().SetBody(body).
		Put(fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(d.Owner), url.PathEscape(d.Repo), escapedPath))
	if err != nil {
		return err
	}
	if res.IsError() {
		return toErr(res)
	}
	return nil
}

func (d *AtomGit) moveDir(srcRelPath, dstRelPath string) error {
	contents, err := d.listContents(srcRelPath)
	if err != nil {
		return err
	}
	for _, c := range contents {
		srcChild := joinPath(srcRelPath, c.Name)
		dstChild := joinPath(dstRelPath, c.Name)
		if c.Type == "dir" {
			if err := d.moveDir(srcChild, dstChild); err != nil {
				return err
			}
		} else {
			msg := fmt.Sprintf("OpenList: move %s to %s", srcChild, dstChild)
			if err := d.moveFile(srcChild, dstChild, msg); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *AtomGit) relativePath(full string) string {
	full = utils.FixAndCleanPath(full)
	return strings.TrimPrefix(full, "/")
}

func (d *AtomGit) applyProxy(raw string) string {
	if raw == "" || d.DownloadProxy == "" {
		return raw
	}
	proxy := d.DownloadProxy
	if !strings.HasSuffix(proxy, "/") {
		proxy += "/"
	}
	return proxy + strings.TrimLeft(raw, "/")
}

func (d *AtomGit) addCommitterAndAuthor(m map[string]string) {
	if d.CommitterName != "" {
		m["committer_name"] = d.CommitterName
		if d.CommitterEmail != "" {
			m["committer_email"] = d.CommitterEmail
		}
	}
	if d.AuthorName != "" {
		m["author_name"] = d.AuthorName
		if d.AuthorEmail != "" {
			m["author_email"] = d.AuthorEmail
		}
	}
}

func encodePath(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return strings.TrimPrefix(stdpath.Join(base, name), "./")
}

var _ driver.Driver = (*AtomGit)(nil)
