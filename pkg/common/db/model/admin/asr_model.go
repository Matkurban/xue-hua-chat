package admin

import (
	"context"

	"chat/pkg/common/db/table/admin"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewAsrModel(db *mongo.Database) (admin.AsrModelInterface, error) {
	coll := db.Collection("asr_model")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "updateTime", Value: -1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &AsrModelMgo{coll: coll}, nil
}

type AsrModelMgo struct {
	coll *mongo.Collection
}

func (o *AsrModelMgo) List(ctx context.Context) ([]*admin.AsrModel, error) {
	return mongoutil.Find[*admin.AsrModel](ctx, o.coll, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
}

func (o *AsrModelMgo) Upsert(ctx context.Context, val *admin.AsrModel) error {
	if val == nil || val.ID == "" {
		return errs.ErrArgs.WrapMsg("asr model id is empty")
	}
	_, err := o.coll.ReplaceOne(ctx, bson.M{"_id": val.ID}, val, options.Replace().SetUpsert(true))
	return errs.Wrap(err)
}
