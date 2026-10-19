package admin

import (
	"context"
	"time"

	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Application is one published client package (one document per platform/arch/version/build).
//
// "latest" is no longer stored: it is computed as the highest (version, build) among the
// published records matching the client's platform and arch.
type Application struct {
	ID          bson.ObjectID `bson:"_id"`
	Platform    string        `bson:"platform"`     // android|ios|windows|macos|linux
	Arch        string        `bson:"arch"`         // universal|arm64|x64|armeabi-v7a|amd64
	Version     string        `bson:"version"`      // semver x.y.z
	Build       int64         `bson:"build"`        // build number, tie-breaker for equal versions
	Url         string        `bson:"url"`          // external package URL, may be empty for ios
	Size        int64         `bson:"size"`         // bytes
	Sha256      string        `bson:"sha256"`       // lowercase hex
	PackageType string        `bson:"package_type"` // apk|exe|dmg|zip|deb|ipa|tar.gz
	Text        string        `bson:"text"`         // release notes
	Force       bool          `bson:"force"`        // force update for every older client
	MinVersion  string        `bson:"min_version"`  // clients below this version are forced
	Status      string        `bson:"status"`       // published|revoked
	CreateTime  time.Time     `bson:"create_time"`
}

type ApplicationInterface interface {
	// ListPublished returns all published records of a platform (any arch).
	ListPublished(ctx context.Context, platform string) ([]*Application, error)
	Take(ctx context.Context, id bson.ObjectID) (*Application, error)
	AddVersion(ctx context.Context, val *Application) error
	UpdateVersion(ctx context.Context, id bson.ObjectID, update map[string]any) error
	DeleteVersion(ctx context.Context, id []bson.ObjectID) error
	PageVersion(ctx context.Context, platforms []string, page pagination.Pagination) (int64, []*Application, error)
	FindPlatform(ctx context.Context, id []bson.ObjectID) ([]string, error)
}
