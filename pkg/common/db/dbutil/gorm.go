package dbutil

import (
	"gitee.com/matkurban/openimsdk-tools/errs"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func IsDBNotFound(err error) bool {
	return errs.Unwrap(err) == mongo.ErrNoDocuments
}
