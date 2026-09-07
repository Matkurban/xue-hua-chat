package admin

import (
	"context"
	"errors"
	"strings"

	"chat/pkg/common/db/table/admin"
	"gitee.com/matkurban/openimsdk-tools/db/mongoutil"
	"gitee.com/matkurban/openimsdk-tools/db/pagination"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func isDupKey(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if mongo.IsDuplicateKeyError(e) {
			return true
		}
		if strings.Contains(e.Error(), "E11000") {
			return true
		}
	}
	return false
}

func NewSystemNotification(db *mongo.Database) (admin.SystemNotificationInterface, error) {
	coll := db.Collection("system_notifications")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "notification_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "create_time", Value: -1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &SystemNotification{coll: coll}, nil
}

type SystemNotification struct {
	coll *mongo.Collection
}

func (o *SystemNotification) Create(ctx context.Context, n *admin.SystemNotification) error {
	if n == nil {
		return errs.ErrArgs.WrapMsg("notification is nil")
	}
	err := mongoutil.InsertOne(ctx, o.coll, n)
	if isDupKey(err) {
		return nil
	}
	return err
}

func (o *SystemNotification) Take(ctx context.Context, notificationID string) (*admin.SystemNotification, error) {
	return mongoutil.FindOne[*admin.SystemNotification](ctx, o.coll, bson.M{"notification_id": notificationID})
}

func (o *SystemNotification) Update(ctx context.Context, notificationID string, update map[string]any) error {
	if len(update) == 0 {
		return nil
	}
	return mongoutil.UpdateOne(ctx, o.coll, bson.M{"notification_id": notificationID}, bson.M{"$set": update}, true)
}

func (o *SystemNotification) Search(ctx context.Context, pagination pagination.Pagination) (int64, []*admin.SystemNotification, error) {
	opt := options.Find().SetSort(bson.D{{Key: "create_time", Value: -1}})
	return mongoutil.FindPage[*admin.SystemNotification](ctx, o.coll, bson.M{}, pagination, opt)
}

func NewSystemNotificationTarget(db *mongo.Database) (admin.SystemNotificationTargetInterface, error) {
	coll := db.Collection("system_notification_targets")
	_, err := coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "notification_id", Value: 1}, {Key: "recv_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "notification_id", Value: 1}, {Key: "status", Value: 1}}},
	})
	if err != nil {
		return nil, errs.Wrap(err)
	}
	return &SystemNotificationTarget{coll: coll}, nil
}

type SystemNotificationTarget struct {
	coll *mongo.Collection
}

func (o *SystemNotificationTarget) CreateMany(ctx context.Context, items []*admin.SystemNotificationTarget) error {
	if len(items) == 0 {
		return nil
	}
	const chunk = 1000
	for i := 0; i < len(items); i += chunk {
		end := i + chunk
		if end > len(items) {
			end = len(items)
		}
		if err := mongoutil.InsertMany(ctx, o.coll, items[i:end]); err != nil {
			if isDupKey(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func (o *SystemNotificationTarget) ListByNotification(ctx context.Context, notificationID string, skip, limit int64) ([]*admin.SystemNotificationTarget, error) {
	opt := options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "recv_id", Value: 1}})
	return mongoutil.Find[*admin.SystemNotificationTarget](ctx, o.coll, bson.M{"notification_id": notificationID}, opt)
}

func (o *SystemNotificationTarget) CountByNotification(ctx context.Context, notificationID string, status string) (int64, error) {
	filter := bson.M{"notification_id": notificationID}
	if status != "" {
		filter["status"] = status
	}
	return mongoutil.Count(ctx, o.coll, filter)
}

func (o *SystemNotificationTarget) MarkStatus(ctx context.Context, notificationID string, recvIDs []string, status string) error {
	if len(recvIDs) == 0 {
		return nil
	}
	_, err := mongoutil.UpdateMany(ctx, o.coll, bson.M{
		"notification_id": notificationID,
		"recv_id":         bson.M{"$in": recvIDs},
	}, bson.M{"$set": bson.M{"status": status}})
	return err
}
