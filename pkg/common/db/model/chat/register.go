package chat

import (
	"context"
	"time"

	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"chat/pkg/common/db/table/chat"
	"gitee.com/matkurban/openimsdk-tools/errs"
)

func NewRegister(db *mongo.Database) (chat.RegisterInterface, error) {
	coll := db.Collection("register")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &Register{coll: coll}, nil
}

type Register struct {
	coll *mongo.Collection
}

func (o *Register) Create(ctx context.Context, registers ...*chat.Register) error {
	return mongoutil.InsertMany(ctx, o.coll, registers)
}

func (o *Register) CountTotal(ctx context.Context, before *time.Time) (int64, error) {
	filter := bson.M{}
	if before != nil {
		filter["create_time"] = bson.M{"$lt": before}
	}
	return mongoutil.Count(ctx, o.coll, filter)
}

func (o *Register) Delete(ctx context.Context, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	return mongoutil.DeleteMany(ctx, o.coll, bson.M{"user_id": bson.M{"$in": userIDs}})
}

func (o *Register) FindCreateTimes(ctx context.Context, userIDs []string) (map[string]time.Time, error) {
	res := make(map[string]time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return res, nil
	}
	list, err := mongoutil.Find[*chat.Register](ctx, o.coll, bson.M{"user_id": bson.M{"$in": userIDs}})
	if err != nil {
		return nil, err
	}
	for _, v := range list {
		res[v.UserID] = v.CreateTime
	}
	return res, nil
}
