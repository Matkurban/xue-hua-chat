package admin

import (
	"context"

	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/db/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"chat/pkg/common/db/table/admin"
	"chat/pkg/common/errs"
)

func NewRegisterAddGroup(db *mongo.Database) (admin.RegisterAddGroupInterface, error) {
	coll := db.Collection("register_add_group")
	_, err := coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{
			{Key: "group_id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &RegisterAddGroup{
		coll: coll,
	}, nil
}

type RegisterAddGroup struct {
	coll *mongo.Collection
}

func (o *RegisterAddGroup) Add(ctx context.Context, registerAddGroups []*admin.RegisterAddGroup) error {
	return mongoutil.InsertMany(ctx, o.coll, registerAddGroups)
}

func (o *RegisterAddGroup) Del(ctx context.Context, groupIDs []string) error {
	if len(groupIDs) == 0 {
		return nil
	}
	return mongoutil.DeleteMany(ctx, o.coll, bson.M{"group_id": bson.M{"$in": groupIDs}})
}

func (o *RegisterAddGroup) FindGroupID(ctx context.Context, groupIDs []string) ([]string, error) {
	filter := bson.M{}
	if len(groupIDs) > 0 {
		filter["group_id"] = bson.M{"$in": groupIDs}
	}
	return mongoutil.Find[string](ctx, o.coll, filter, options.Find().SetProjection(bson.M{"_id": 0, "group_id": 1}))
}

func (o *RegisterAddGroup) Search(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*admin.RegisterAddGroup, error) {
	filter := bson.M{"group_id": bson.M{"$regex": keyword, "$options": "i"}}
	return mongoutil.FindPage[*admin.RegisterAddGroup](ctx, o.coll, filter, pagination)
}
