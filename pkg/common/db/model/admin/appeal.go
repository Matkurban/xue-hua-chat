package admin

import (
	"context"
	"time"

	"chat/pkg/common/db/table/admin"
	"chat/pkg/common/db/mongoutil"
	"chat/pkg/common/db/pagination"
	"chat/pkg/common/errs"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewAppeal(db *mongo.Database) (admin.AppealInterface, error) {
	coll := db.Collection("appeals")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "appeal_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "create_time", Value: -1}}},
		{Keys: bson.D{{Key: "account", Value: 1}}},
		{Keys: bson.D{{Key: "client_ip", Value: 1}, {Key: "create_time", Value: -1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &Appeal{coll: coll}, nil
}

type Appeal struct {
	coll *mongo.Collection
}

func (o *Appeal) Create(ctx context.Context, ms []*admin.Appeal) error {
	return mongoutil.InsertMany(ctx, o.coll, ms)
}

func (o *Appeal) Take(ctx context.Context, appealID string) (*admin.Appeal, error) {
	return mongoutil.FindOne[*admin.Appeal](ctx, o.coll, bson.M{"appeal_id": appealID})
}

func (o *Appeal) Update(ctx context.Context, appealID string, update map[string]any) error {
	if len(update) == 0 {
		return nil
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"appeal_id": appealID}, bson.M{"$set": update}, true)
}

func (o *Appeal) AddEvidence(ctx context.Context, appealID string, url string) error {
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"appeal_id": appealID}, bson.M{"$push": bson.M{"evidence": url}}, true)
}

func (o *Appeal) Search(ctx context.Context, status string, keyword string, pagination pagination.Pagination) (int64, []*admin.Appeal, error) {
	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}
	if keyword != "" {
		filter["$or"] = []bson.M{
			{"account": bson.M{"$regex": keyword, "$options": "i"}},
			{"user_id": bson.M{"$regex": keyword, "$options": "i"}},
			{"description": bson.M{"$regex": keyword, "$options": "i"}},
		}
	}
	opt := options.Find().SetSort(bson.M{"create_time": -1})
	return mongoutil.FindPage[*admin.Appeal](ctx, o.coll, filter, pagination, opt)
}

func (o *Appeal) CountRecent(ctx context.Context, account string, ip string, since time.Time) (int64, error) {
	or := []bson.M{}
	if account != "" {
		or = append(or, bson.M{"account": account})
	}
	if ip != "" {
		or = append(or, bson.M{"client_ip": ip})
	}
	if len(or) == 0 {
		return 0, nil
	}
	filter := bson.M{
		"$or":         or,
		"create_time": bson.M{"$gte": since},
	}
	return mongoutil.Count(ctx, o.coll, filter)
}
