package rtc

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var meetingPasswordRe = regexp.MustCompile(`^[A-Za-z0-9]{4,16}$`)

const (
	passwordFailKeyPrefix = "schedule:pwd_fail:"
	passwordFailMax       = 10
	passwordFailTTL       = 5 * time.Minute
)

// ValidateMeetingPasswordFormat 校验明文密码格式；空字符串表示不设密。
func ValidateMeetingPasswordFormat(password string) error {
	if password == "" {
		return nil
	}
	if !meetingPasswordRe.MatchString(password) {
		return fmt.Errorf("password must be 4-16 alphanumeric characters")
	}
	return nil
}

// HashMeetingPassword 生成 bcrypt 哈希。
func HashMeetingPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyMeetingPassword 校验明文与哈希。
func VerifyMeetingPassword(hash, password string) bool {
	if hash == "" {
		return password == ""
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// PasswordRateLimiter 密码错误限流。
type PasswordRateLimiter struct {
	rdb redis.UniversalClient
}

func NewPasswordRateLimiter(rdb redis.UniversalClient) *PasswordRateLimiter {
	return &PasswordRateLimiter{rdb: rdb}
}

func (l *PasswordRateLimiter) key(meetingID, userID string) string {
	return passwordFailKeyPrefix + meetingID + ":" + userID
}

// CheckAllowed 是否允许继续尝试验证密码。
func (l *PasswordRateLimiter) CheckAllowed(ctx context.Context, meetingID, userID string) (bool, error) {
	if l == nil || l.rdb == nil {
		return true, nil
	}
	n, err := l.rdb.Get(ctx, l.key(meetingID, userID)).Int()
	if err == redis.Nil {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return n < passwordFailMax, nil
}

// RecordFailure 记录一次失败。
func (l *PasswordRateLimiter) RecordFailure(ctx context.Context, meetingID, userID string) {
	if l == nil || l.rdb == nil {
		return
	}
	k := l.key(meetingID, userID)
	n, err := l.rdb.Incr(ctx, k).Result()
	if err != nil {
		return
	}
	if n == 1 {
		_ = l.rdb.Expire(ctx, k, passwordFailTTL).Err()
	}
}

// ClearFailures 验证成功后清除计数。
func (l *PasswordRateLimiter) ClearFailures(ctx context.Context, meetingID, userID string) {
	if l == nil || l.rdb == nil {
		return
	}
	_ = l.rdb.Del(ctx, l.key(meetingID, userID)).Err()
}
