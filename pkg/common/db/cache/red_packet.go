package cache

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// pointsScale 积分在 Redis 中以整数保存（×100），避免 Lua 脚本浮点精度问题。
// 例如 1.50 积分存为 150，读出后除以 100 还原。
const pointsScale = 100.0

const (
	redPacketAmountsKey = "rp:amounts:%s" // LRANGE key: 存预分配金额列表
	redPacketGrabbedKey = "rp:grabbed:%s" // SET key: 已领取的 userID 集合
	redPacketTTL        = 26 * time.Hour  // 比红包24小时有效期多2小时缓冲
)

// grabLuaScript 原子抢红包脚本
// KEYS[1]: amounts list key
// KEYS[2]: grabbed set key
// ARGV[1]: grabberID
// ARGV[2]: grabbed set TTL in seconds
// 返回: 领取金额(>0) | 0=已领过 | -1=已抢完
const grabLuaScript = `
local already = redis.call('SISMEMBER', KEYS[2], ARGV[1])
if already == 1 then return 0 end
local amount = redis.call('LPOP', KEYS[1])
if not amount then return -1 end
redis.call('SADD', KEYS[2], ARGV[1])
redis.call('EXPIRE', KEYS[2], tonumber(ARGV[2]))
return tonumber(amount)
`

// rollbackGrabLuaScript DB 落库失败时回滚 Redis 抢红包状态
// KEYS[1]: amounts list key
// KEYS[2]: grabbed set key
// ARGV[1]: grabberID
// ARGV[2]: scaled amount
// ARGV[3]: TTL in seconds
const rollbackGrabLuaScript = `
local removed = redis.call('SREM', KEYS[2], ARGV[1])
if removed == 1 then
    redis.call('LPUSH', KEYS[1], ARGV[2])
    redis.call('EXPIRE', KEYS[1], tonumber(ARGV[3]))
end
return removed
`

// RedPacketCache 红包 Redis 缓存操作
type RedPacketCache struct {
	rdb            redis.UniversalClient
	script         *redis.Script
	rollbackScript *redis.Script
}

func NewRedPacketCache(rdb redis.UniversalClient) *RedPacketCache {
	return &RedPacketCache{
		rdb:            rdb,
		script:         redis.NewScript(grabLuaScript),
		rollbackScript: redis.NewScript(rollbackGrabLuaScript),
	}
}

// InitPacket 预分配红包金额列表并写入 Redis（创建红包时调用）
// 金额乘以 pointsScale 后以整数存储，方便 Lua 脚本返回。
func (c *RedPacketCache) InitPacket(ctx context.Context, packetID string, totalAmount float64, count int32, luckDraw bool) error {
	amounts := splitAmounts(totalAmount, count, luckDraw)
	amountsKey := fmt.Sprintf(redPacketAmountsKey, packetID)

	pipe := c.rdb.Pipeline()
	args := make([]interface{}, len(amounts))
	for i, a := range amounts {
		args[i] = int64(math.Round(a * pointsScale))
	}
	pipe.RPush(ctx, amountsKey, args...)
	pipe.Expire(ctx, amountsKey, redPacketTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// GrabPacket 原子抢红包，返回领取金额；0=已领过，-1=已抢完
func (c *RedPacketCache) GrabPacket(ctx context.Context, packetID string, grabberID string) (float64, error) {
	amountsKey := fmt.Sprintf(redPacketAmountsKey, packetID)
	grabbedKey := fmt.Sprintf(redPacketGrabbedKey, packetID)

	scaled, err := c.script.Run(ctx, c.rdb, []string{amountsKey, grabbedKey}, grabberID, int64(redPacketTTL/time.Second)).Int64()
	if err != nil {
		return 0, err
	}
	if scaled == 0 {
		return 0, nil // 已领过
	}
	if scaled == -1 {
		return -1, nil // 已抓完
	}
	return float64(scaled) / pointsScale, nil
}

// RollbackGrab 在数据库事务失败时原子回滚 Redis 抢红包状态，防止金额凭空丢失
func (c *RedPacketCache) RollbackGrab(ctx context.Context, packetID string, grabberID string, amount float64) error {
	amountsKey := fmt.Sprintf(redPacketAmountsKey, packetID)
	grabbedKey := fmt.Sprintf(redPacketGrabbedKey, packetID)
	scaled := int64(math.Round(amount * pointsScale))
	return c.rollbackScript.Run(ctx, c.rdb, []string{amountsKey, grabbedKey}, grabberID, scaled, int64(redPacketTTL/time.Second)).Err()
}

// RemainingCount 查询 Redis List 剩余份数（用于状态校验）
func (c *RedPacketCache) RemainingCount(ctx context.Context, packetID string) (int64, error) {
	return c.rdb.LLen(ctx, fmt.Sprintf(redPacketAmountsKey, packetID)).Result()
}

// DeletePacket 过期/领完后清理 Redis 缓存
func (c *RedPacketCache) DeletePacket(ctx context.Context, packetID string) {
	c.rdb.Del(ctx,
		fmt.Sprintf(redPacketAmountsKey, packetID),
		fmt.Sprintf(redPacketGrabbedKey, packetID),
	)
}

// splitAmounts 使用整数分（×100）精确分割红包金额，保证：
// 1. 每份 >= 0.01（1 分），绝不出现 0 或负数；
// 2. 所有份之和严格等于 total（无浮点累积误差）。
func splitAmounts(total float64, count int32, luckDraw bool) []float64 {
	if count <= 0 {
		return nil
	}
	totalCents := int64(math.Round(total * pointsScale))
	n := int64(count)
	cents := make([]int64, n)

	if !luckDraw {
		base := totalCents / n
		rem := totalCents % n
		for i := int64(0); i < n; i++ {
			cents[i] = base
			if i < rem {
				cents[i]++
			}
		}
	} else {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		remaining := totalCents
		for i := int64(0); i < n-1; i++ {
			leftCount := n - i
			// 每份至少 1 分，同时保证剩余 (leftCount-1) 份每人至少 1 分
			hardMax := remaining - (leftCount - 1)
			doubleAvg := (2 * remaining) / leftCount
			maxCents := doubleAvg
			if maxCents > hardMax {
				maxCents = hardMax
			}
			if maxCents < 1 {
				maxCents = 1
			}
			var pick int64
			if maxCents == 1 {
				pick = 1
			} else {
				pick = 1 + rng.Int63n(maxCents)
			}
			cents[i] = pick
			remaining -= pick
		}
		cents[n-1] = remaining
		rng.Shuffle(int(n), func(i, j int) {
			cents[i], cents[j] = cents[j], cents[i]
		})
	}

	amounts := make([]float64, n)
	for i, c := range cents {
		amounts[i] = float64(c) / pointsScale
	}
	return amounts
}
