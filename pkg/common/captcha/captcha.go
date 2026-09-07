package captcha

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const captchaKeyPrefix = "appeal:captcha:"

// Service 简易数学验证码服务（v1，无图片依赖）
type Service struct {
	rdb redis.UniversalClient
	ttl time.Duration
}

func New(rdb redis.UniversalClient, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Service{rdb: rdb, ttl: ttl}
}

// Challenge 返回给前端的验证码
type Challenge struct {
	CaptchaID string `json:"captchaID"`
	Question  string `json:"question"`  // 形如 "3 + 5 = ?"
	Base64Img string `json:"base64Img"` // v1 留空，v2 可加图片
}

// Generate 生成新的数学验证码并存入 redis
func (s *Service) Generate(ctx context.Context) (*Challenge, error) {
	a, err := randInt(1, 9)
	if err != nil {
		return nil, err
	}
	b, err := randInt(1, 9)
	if err != nil {
		return nil, err
	}
	answer := strconv.Itoa(a + b)
	id, err := randomID(16)
	if err != nil {
		return nil, err
	}
	if err := s.rdb.Set(ctx, captchaKeyPrefix+id, answer, s.ttl).Err(); err != nil {
		return nil, err
	}
	return &Challenge{
		CaptchaID: id,
		Question:  fmt.Sprintf("%d + %d = ?", a, b),
	}, nil
}

// Verify 校验验证码（一次性，校验后删除）
func (s *Service) Verify(ctx context.Context, captchaID, answer string) bool {
	if captchaID == "" || answer == "" {
		return false
	}
	key := captchaKeyPrefix + captchaID
	expected, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	_ = s.rdb.Del(ctx, key).Err()
	return expected == answer
}

func randInt(min, max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + min, nil
}

func randomID(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
