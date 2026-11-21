package mw

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"chat/pkg/common/apiresp"
	"chat/pkg/common/errs"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	appealTokenKeyPrefix = "appeal:token:"
	appealTokenCtxKey    = "appealID"
)

// AppealTokenManager 申诉短期令牌管理器（基于 Redis）
type AppealTokenManager struct {
	rdb redis.UniversalClient
	ttl time.Duration
}

func NewAppealTokenManager(rdb redis.UniversalClient, ttl time.Duration) *AppealTokenManager {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &AppealTokenManager{rdb: rdb, ttl: ttl}
}

// Issue 为申诉发放短期 token
func (a *AppealTokenManager) Issue(ctx context.Context, appealID string) (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	if err := a.rdb.Set(ctx, appealTokenKeyPrefix+token, appealID, a.ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// CheckAppealToken Gin 中间件：校验 Authorization: appeal {token}，把 appealID 注入 context
func (a *AppealTokenManager) CheckAppealToken(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	const prefix = "appeal "
	if !strings.HasPrefix(auth, prefix) {
		// 兼容直接放在 token header 的情况
		if t := c.GetHeader("appealToken"); t != "" {
			auth = prefix + t
		} else {
			c.Abort()
			apiresp.GinError(c, errs.ErrArgs.WrapMsg("appeal token required"))
			return
		}
	}
	token := strings.TrimPrefix(auth, prefix)
	appealID, err := a.rdb.Get(c, appealTokenKeyPrefix+token).Result()
	if err != nil || appealID == "" {
		c.Abort()
		apiresp.GinError(c, errs.ErrTokenExpired.WrapMsg("invalid appeal token"))
		return
	}
	c.Set(appealTokenCtxKey, appealID)
}

// GetAppealID 从 gin.Context 取出申诉 ID
func GetAppealID(c *gin.Context) string {
	v, _ := c.Get(appealTokenCtxKey)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
