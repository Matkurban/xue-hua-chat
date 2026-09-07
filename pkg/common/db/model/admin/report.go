package admin

import (
	"context"

	"chat/pkg/common/db/table/admin"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewReport(db *mongo.Database) (admin.ReportInterface, error) {
	coll := db.Collection("reports")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "report_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "create_time", Value: -1}}},
		{Keys: bson.D{{Key: "reporter_id", Value: 1}}},
		{Keys: bson.D{{Key: "target_id", Value: 1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &Report{coll: coll}, nil
}

type Report struct {
	coll *mongo.Collection
}

func (o *Report) Create(ctx context.Context, ms []*admin.Report) error {
	return mongoutil.InsertMany(ctx, o.coll, ms)
}

func (o *Report) Take(ctx context.Context, reportID string) (*admin.Report, error) {
	return mongoutil.FindOne[*admin.Report](ctx, o.coll, bson.M{"report_id": reportID})
}

func (o *Report) Update(ctx context.Context, reportID string, update map[string]any) error {
	if len(update) == 0 {
		return nil
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"report_id": reportID}, bson.M{"$set": update}, true)
}

func (o *Report) Search(ctx context.Context, status string, category string, keyword string, pagination pagination.Pagination) (int64, []*admin.Report, error) {
	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}
	if category != "" {
		filter["category"] = category
	}
	if keyword != "" {
		filter["$or"] = []bson.M{
			{"reporter_id": bson.M{"$regex": keyword, "$options": "i"}},
			{"target_id": bson.M{"$regex": keyword, "$options": "i"}},
			{"description": bson.M{"$regex": keyword, "$options": "i"}},
		}
	}
	opt := options.Find().SetSort(bson.M{"create_time": -1})
	return mongoutil.FindPage[*admin.Report](ctx, o.coll, filter, pagination, opt)
}
