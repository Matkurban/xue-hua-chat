package admin

import (
	"context"

	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"chat/pkg/common/constant"
	"chat/pkg/common/db/table/admin"
	"gitee.com/matkurban/openimsdk-tools/errs"
)

func NewApplet(db *mongo.Database) (admin.AppletInterface, error) {
	coll := db.Collection("applet")
	_, err := coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{
			{Key: "id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &Applet{
		coll: coll,
	}, nil
}

type Applet struct {
	coll *mongo.Collection
}

func (o *Applet) Create(ctx context.Context, applets []*admin.Applet) error {
	return mongoutil.InsertMany(ctx, o.coll, applets)
}

func (o *Applet) Del(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return mongoutil.DeleteMany(ctx, o.coll, bson.M{"id": bson.M{"$in": ids}})
}

func (o *Applet) Update(ctx context.Context, id string, data map[string]any) error {
	if len(data) == 0 {
		return nil
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"id": id}, bson.M{"$set": data}, false)
}

func (o *Applet) Take(ctx context.Context, id string) (*admin.Applet, error) {
	return mongoutil.FindOne[*admin.Applet](ctx, o.coll, bson.M{"id": id})
}

func (o *Applet) Search(ctx context.Context, keyword string, pagination pagination.Pagination) (int64, []*admin.Applet, error) {
	filter := bson.M{}

	if keyword != "" {
		filter = bson.M{
			"$or": []bson.M{
				{"name": bson.M{"$regex": keyword, "$options": "i"}},
				{"id": bson.M{"$regex": keyword, "$options": "i"}},
				{"app_id": bson.M{"$regex": keyword, "$options": "i"}},
				{"version": bson.M{"$regex": keyword, "$options": "i"}},
			},
		}
	}
	return mongoutil.FindPage[*admin.Applet](ctx, o.coll, filter, pagination)
}

func (o *Applet) FindOnShelf(ctx context.Context) ([]*admin.Applet, error) {
	return mongoutil.Find[*admin.Applet](ctx, o.coll, bson.M{"status": constant.StatusOnShelf})
}

func (o *Applet) FindID(ctx context.Context, ids []string) ([]*admin.Applet, error) {
	return mongoutil.Find[*admin.Applet](ctx, o.coll, bson.M{"id": bson.M{"$in": ids}})
}
