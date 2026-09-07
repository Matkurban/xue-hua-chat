package admin

import (
	"context"
	"time"
)

// AsrModelFile 单个需下载的模型文件。
type AsrModelFile struct {
	FileName string `bson:"fileName" json:"fileName"`
	Url      string `bson:"url" json:"url"`
	MinBytes int64  `bson:"minBytes" json:"minBytes"`
	Sha256   string `bson:"sha256" json:"sha256"`
}

// AsrModel 后台配置的离线 ASR 模型清单。
type AsrModel struct {
	ID              string         `bson:"_id" json:"id"`
	Version         string         `bson:"version" json:"version"`
	ApproxSizeBytes int64          `bson:"approxSizeBytes" json:"approxSizeBytes"`
	Files           []AsrModelFile `bson:"files" json:"files"`
	UpdateTime      time.Time      `bson:"updateTime" json:"updateTime"`
}

func (AsrModel) TableName() string {
	return "asr_model"
}

type AsrModelInterface interface {
	List(ctx context.Context) ([]*AsrModel, error)
	Upsert(ctx context.Context, val *AsrModel) error
}
