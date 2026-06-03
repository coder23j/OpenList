package nami_kb

import (
	"strconv"
	"time"
)

type commonResp struct {
	Errno  int    `json:"errno"`
	Errmsg string `json:"errmsg"`
}

type tokenResp struct {
	commonResp
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type folderItem struct {
	FolderID string `json:"folder_id"`
	Name     string `json:"name"`
}

type folderListResp struct {
	commonResp
	Data struct {
		List []folderItem `json:"list"`
	} `json:"data"`
}

type fileItem struct {
	FileID     string `json:"file_id"`
	Name       string `json:"name"`
	Size       string `json:"size"`
	ShareQid   string `json:"share_qid"`
	FileHash   string `json:"file_hash"`
	ModifyTime string `json:"modify_time"`
	CreateTime string `json:"create_time"`
}

func (f *fileItem) SizeInt() int64 {
	n, _ := strconv.ParseInt(f.Size, 10, 64)
	return n
}

func (f *fileItem) ModifyTimeInt() int64 {
	n, _ := strconv.ParseInt(f.ModifyTime, 10, 64)
	return n
}

func (f *fileItem) CreateTimeInt() int64 {
	n, _ := strconv.ParseInt(f.CreateTime, 10, 64)
	return n
}

type fileListResp struct {
	commonResp
	Data struct {
		List  []fileItem `json:"list"`
		Total int        `json:"total"`
	} `json:"data"`
}

type downloadResp struct {
	commonResp
	Data struct {
		DownloadURL string `json:"downloadUrl"`
	} `json:"data"`
}

func unixOrZero(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}
