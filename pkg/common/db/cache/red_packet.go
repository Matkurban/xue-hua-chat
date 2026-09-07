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
// 返回: 领取金额(>0) | 0=已领过 | -1=已抢完
const grabLuaScript = `
local already = redis.call('SISMEMBER', KEYS[2], ARGV[1])
if already == 1 then return 0 end
local amount = redis.call('LPOP', KEYS[1])
if not amount then return -1 end
redis.call('SADD', KEYS[2], ARGV[1])
return tonumber(amount)
`

// RedPacketCache 红包 Redis 缓存操作
type RedPacketCache struct {
	rdb    redis.UniversalClient
	script *redis.Script
}

func NewRedPacketCache(rdb redis.UniversalClient) *RedPacketCache {
	return &RedPacketCache{
		rdb:    rdb,
		script: redis.NewScript(grabLuaScript),
	}
}

// InitPacket 预分配红包金额列表并写入 Redis（创建红包时调用）
// 金额乘以 pointsScale 后以整数存储，方便 Lua 脚本返回。
func (c *RedPacketCache) InitPacket(ctx context.Context, packetID string, totalAmount float64, count int32, luckDraw bool) error {
	amounts := splitAmounts(totalAmount, count, luckDraw)
	amountsKey := fmt.Sprintf(redPacketAmountsKey, packetID)
	grabbedKey := fmt.Sprintf(redPacketGrabbedKey, packetID)

	pipe := c.rdb.Pipeline()
	// 将金额列表推入 Redis List（以整数存储）
	args := make([]interface{}, len(amounts))
	for i, a := range amounts {
		args[i] = int64(math.Round(a * pointsScale))
	}
	pipe.RPush(ctx, amountsKey, args...)
	pipe.Expire(ctx, amountsKey, redPacketTTL)
	pipe.Expire(ctx, grabbedKey, redPacketTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// GrabPacket 原子抢红包，返回领取金额；0=已领过，-1=已抢完
func (c *RedPacketCache) GrabPacket(ctx context.Context, packetID string, grabberID string) (float64, error) {
	amountsKey := fmt.Sprintf(redPacketAmountsKey, packetID)
	grabbedKey := fmt.Sprintf(redPacketGrabbedKey, packetID)

	scaled, err := c.script.Run(ctx, c.rdb, []string{amountsKey, grabbedKey}, grabberID).Int64()
	if err != nil {
		return 0, err
	}
	if scaled == 0 {
		return 0, nil // 已领过
	}
	if scaled == -1 {
		return -1, nil // 已抓完
	}
	// scaled 已经是乘以 100 的整数，除以 100 即可还原到两位小数
	return float64(scaled) / pointsScale, nil
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

// splitAmounts 拼手气随机分割；均等时等额分割
// 拼手气算法：二倍均值法，保证每份 >= 0.01，且随机性好
// 返回的金额保留 2 位小数。
func splitAmounts(total float64, count int32, luckDraw bool) []float64 {
	const minAmount = 0.01
	amounts := make([]float64, count)
	if !luckDraw {
		base := math.Round(total/float64(count)*100) / 100
		for i := int32(0); i < count-1; i++ {
			amounts[i] = base
		}
		// 最后一份 = 总金额 - 前 n-1 份之和，避免浮点累积误差
		var sum float64
		for i := int32(0); i < count-1; i++ {
			sum += amounts[i]
		}
		amounts[count-1] = math.Round((total-sum)*100) / 100
		return amounts
	}

	// 拼手气：二倍均值法
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	remaining := total
	for i := int32(0); i < count-1; i++ {
		leftCount := float64(count - i)
		// 每份最大值 = 2*remaining/leftCount，确保剩余份至少能分到 minAmount
		maxAmount := 2*remaining/leftCount - minAmount*(leftCount-1)/leftCount
		if maxAmount < minAmount {
			maxAmount = minAmount
		}
		amount := math.Round((minAmount+rng.Float64()*(maxAmount-minAmount))*100) / 100
		amounts[i] = amount
		remaining = math.Round((remaining-amount)*100) / 100
	}
	amounts[count-1] = math.Round(remaining*100) / 100
	// 打乱顺序增加随机感
	rng.Shuffle(int(count), func(i, j int) {
		amounts[i], amounts[j] = amounts[j], amounts[i]
	})
	return amounts
}
