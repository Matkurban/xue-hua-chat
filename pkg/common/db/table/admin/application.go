package admin

import (
	"context"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"time"
)

type Application struct {
	CreateTime time.Time     `bson:"create_time"`
	Platform   string        `bson:"platform"`
	Version    string        `bson:"version"`
	Url        string        `bson:"url"`
	Text       string        `bson:"text"`
	ID         bson.ObjectID `bson:"_id"`
	Hot        bool          `bson:"hot"`
	Force      bool          `bson:"force"`
	Latest     bool          `bson:"latest"`
}

type ApplicationInterface interface {
	LatestVersion(ctx context.Context, platform string) (*Application, error)
	AddVersion(ctx context.Context, val *Application) error
	UpdateVersion(ctx context.Context, id bson.ObjectID, update map[string]any) error
	DeleteVersion(ctx context.Context, id []bson.ObjectID) error
	PageVersion(ctx context.Context, platforms []string, page pagination.Pagination) (int64, []*Application, error)
	FindPlatform(ctx context.Context, id []bson.ObjectID) ([]string, error)
}
