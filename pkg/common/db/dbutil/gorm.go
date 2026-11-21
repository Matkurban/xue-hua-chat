package dbutil

import (
	"chat/pkg/common/errs"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func IsDBNotFound(err error) bool {
	return errs.Unwrap(err) == mongo.ErrNoDocuments
}
