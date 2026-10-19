package admin

import (
	"context"

	admindb "chat/pkg/common/db/table/admin"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// NewApplication opens the `application` collection.
//
// Schema v2 (arch/build/size/sha256/status...) is incompatible with v1 documents
// (platform+version only, latest/hot flags). The old unique index (platform, version) and
// the `latest` index are dropped if present; see docs/app-update.md for the migration.
func NewApplication(db *mongo.Database) (admindb.ApplicationInterface, error) {
	coll := db.Collection("application")
	ctx := context.Background()
	for _, old := range []string{"platform_1_version_1", "latest_-1"} {
		_ = coll.Indexes().DropOne(ctx, old) // ignore: absent on fresh databases
	}
	_, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "platform", Value: 1},
				{Key: "arch", Value: 1},
				{Key: "version", Value: 1},
				{Key: "build", Value: 1},
			},
			Options: options.Index().SetUnique(true).SetName("uniq_release"),
		},
		{
			Keys: bson.D{
				{Key: "platform", Value: 1},
				{Key: "status", Value: 1},
			},
			Options: options.Index().SetName("platform_status"),
		},
	})
	if err != nil {
		return nil, err
	}
	return &ApplicationMgo{coll: coll}, nil
}

type ApplicationMgo struct {
	coll *mongo.Collection
}

func (a *ApplicationMgo) ListPublished(ctx context.Context, platform string) ([]*admindb.Application, error) {
	return mongoutil.Find[*admindb.Application](ctx, a.coll, bson.M{"platform": platform, "status": "published"})
}

func (a *ApplicationMgo) Take(ctx context.Context, id bson.ObjectID) (*admindb.Application, error) {
	return mongoutil.FindOne[*admindb.Application](ctx, a.coll, bson.M{"_id": id})
}

func (a *ApplicationMgo) AddVersion(ctx context.Context, val *admindb.Application) error {
	if val.ID.IsZero() {
		val.ID = bson.NewObjectID()
	}
	return mongoutil.InsertMany(ctx, a.coll, []*admindb.Application{val})
}

func (a *ApplicationMgo) UpdateVersion(ctx context.Context, id bson.ObjectID, update map[string]any) error {
	if len(update) == 0 {
		return nil
	}
	return mongoutil.UpdateOne(ctx, a.coll, bson.M{"_id": id}, bson.M{"$set": update}, true)
}

func (a *ApplicationMgo) DeleteVersion(ctx context.Context, id []bson.ObjectID) error {
	if len(id) == 0 {
		return nil
	}
	return mongoutil.DeleteMany(ctx, a.coll, bson.M{"_id": bson.M{"$in": id}})
}

func (a *ApplicationMgo) PageVersion(ctx context.Context, platforms []string, page pagination.Pagination) (int64, []*admindb.Application, error) {
	filter := bson.M{}
	if len(platforms) > 0 {
		filter["platform"] = bson.M{"$in": platforms}
	}
	return mongoutil.FindPage[*admindb.Application](ctx, a.coll, filter, page, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}))
}

func (a *ApplicationMgo) FindPlatform(ctx context.Context, id []bson.ObjectID) ([]string, error) {
	if len(id) == 0 {
		return nil, nil
	}
	return mongoutil.Find[string](ctx, a.coll, bson.M{"_id": bson.M{"$in": id}}, options.Find().SetProjection(bson.M{"_id": 0, "platform": 1}))
}
