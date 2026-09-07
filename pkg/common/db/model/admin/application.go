package admin

import (
	"chat/pkg/common/db/table/admin"
	admindb "chat/pkg/common/db/table/admin"
	"context"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewApplication(db *mongo.Database) (admindb.ApplicationInterface, error) {
	coll := db.Collection("application")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "platform", Value: 1},
				{Key: "version", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "latest", Value: -1},
			},
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

func (a *ApplicationMgo) sort() any {
	return bson.D{{"latest", -1}, {"_id", -1}}
}

func (a *ApplicationMgo) LatestVersion(ctx context.Context, platform string) (*admin.Application, error) {
	return mongoutil.FindOne[*admin.Application](ctx, a.coll, bson.M{"platform": platform}, options.FindOne().SetSort(a.sort()))
}

func (a *ApplicationMgo) AddVersion(ctx context.Context, val *admin.Application) error {
	if val.ID.IsZero() {
		val.ID = bson.NewObjectID()
	}
	return mongoutil.InsertMany(ctx, a.coll, []*admin.Application{val})
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

func (a *ApplicationMgo) PageVersion(ctx context.Context, platforms []string, page pagination.Pagination) (int64, []*admin.Application, error) {
	filter := bson.M{}
	if len(platforms) > 0 {
		filter["platform"] = bson.M{"$in": platforms}
	}
	return mongoutil.FindPage[*admin.Application](ctx, a.coll, filter, page, options.Find().SetSort(a.sort()))
}

func (a *ApplicationMgo) FindPlatform(ctx context.Context, id []bson.ObjectID) ([]string, error) {
	if len(id) == 0 {
		return nil, nil
	}
	return mongoutil.Find[string](ctx, a.coll, bson.M{"_id": bson.M{"$in": id}}, options.Find().SetProjection(bson.M{"_id": 0, "platform": 1}))
}
