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

type mongoVerifyCode struct {
	CreateTime time.Time     `bson:"create_time"`
	Account    string        `bson:"account"`
	Platform   string        `bson:"platform"`
	Code       string        `bson:"code"`
	Duration   uint          `bson:"duration"`
	Count      int           `bson:"count"`
	ID         bson.ObjectID `bson:"_id"`
	Used       bool          `bson:"used"`
}

func NewVerifyCode(db *mongo.Database) (chat.VerifyCodeInterface, error) {
	coll := db.Collection("verify_code")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "account", Value: 1},
			},
		},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &VerifyCode{
		coll: coll,
	}, nil
}

type VerifyCode struct {
	coll *mongo.Collection
}

func (o *VerifyCode) parseID(s string) (bson.ObjectID, error) {
	objID, err := bson.ObjectIDFromHex(s)
	if err != nil {
		var zero bson.ObjectID
		return zero, errs.Wrap(err)
	}
	return objID, nil
}

func (o *VerifyCode) Add(ctx context.Context, ms []*chat.VerifyCode) error {
	tmp := make([]mongoVerifyCode, 0, len(ms))
	for i, m := range ms {
		var objID bson.ObjectID
		if m.ID == "" {
			objID = bson.NewObjectID()
			ms[i].ID = objID.Hex()
		} else {
			var err error
			objID, err = o.parseID(m.ID)
			if err != nil {
				return err
			}
		}
		tmp = append(tmp, mongoVerifyCode{
			ID:         objID,
			Account:    m.Account,
			Platform:   m.Platform,
			Code:       m.Code,
			Duration:   m.Duration,
			Count:      m.Count,
			Used:       m.Used,
			CreateTime: m.CreateTime,
		})
	}
	return mongoutil.InsertMany(ctx, o.coll, tmp)
}

func (o *VerifyCode) RangeNum(ctx context.Context, account string, start time.Time, end time.Time) (int64, error) {
	filter := bson.M{
		"account": account,
		"create_time": bson.M{
			"$gte": start,
			"$lte": end,
		},
	}
	return mongoutil.Count(ctx, o.coll, filter)
}

func (o *VerifyCode) TakeLast(ctx context.Context, account string) (*chat.VerifyCode, error) {
	filter := bson.M{
		"account": account,
	}
	opt := options.FindOne().SetSort(bson.M{"_id": -1})
	last, err := mongoutil.FindOne[*mongoVerifyCode](ctx, o.coll, filter, opt)
	if err != nil {
		return nil, err
	}
	return &chat.VerifyCode{
		ID:         last.ID.Hex(),
		Account:    last.Account,
		Platform:   last.Platform,
		Code:       last.Code,
		Duration:   last.Duration,
		Count:      last.Count,
		Used:       last.Used,
		CreateTime: last.CreateTime,
	}, nil
}

func (o *VerifyCode) Incr(ctx context.Context, id string) error {
	objID, err := o.parseID(id)
	if err != nil {
		return err
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"_id": objID}, bson.M{"$inc": bson.M{"count": 1}}, false)
}

func (o *VerifyCode) Delete(ctx context.Context, id string) error {
	objID, err := o.parseID(id)
	if err != nil {
		return err
	}
	return mongoutil.DeleteOne(ctx, o.coll, bson.M{"_id": objID})
}
