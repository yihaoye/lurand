package lurand

import (
	"context"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	script = `
		local key = KEYS[1]
		local timeout = tonumber(ARGV[1])
		local max = tonumber(redis.call("GET", key))
		if not max or max <= 0 then
			return redis.error_reply("No more numbers available")
		end

		-- lua math.random range is [1, max]
		local idx = math.random(max) - 1
		local val = redis.call("GET", key .. idx)
		if not val then
			val = idx
		end
		local rep = redis.call("GET", key .. (max - 1))
		if not rep then
			rep = max - 1
		end

		redis.call("SET", key .. idx, rep, "EX", timeout)
		redis.call("SET", key, max - 1, "EX", timeout)
		redis.call("DEL", key .. max)

		return val
	`

	ONE_DAY = 24 * 60 * 60
)

var (
	client  *redis.Client
	timeout int

	once sync.Once
)

func InitCache(addr string) {
	InitCache_(addr, ONE_DAY)
}

func InitCache_(addr string, t int) {
	InitCache__(addr, "", 0, t)
}

func InitCache__(addr, password string, db, t int) {
	once.Do(func() {
		client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
			PoolSize: 10,
		})
		timeout = t
	})
}

type CacheLUR struct {
	key string
	k   int32
}

func NewCacheLUR(ctx context.Context, key string) *CacheLUR {
	return NewCacheLUR__(ctx, key, ONE_MILLION, 1)
}

func NewCacheLUR_(ctx context.Context, key string, max int32) *CacheLUR {
	return NewCacheLUR__(ctx, key, max, 1)
}

func NewCacheLUR__(ctx context.Context, key string, max, k int32) *CacheLUR {
	cmd := client.Set(ctx, key, max, time.Duration(timeout)*time.Second)
	if cmd.Err() != nil {
		panic(cmd.Err())
	}
	return &CacheLUR{
		key: key,
		k:   k,
	}
}

func (r *CacheLUR) Int31n(ctx context.Context) (int32, error) {
	val, err := client.Eval(ctx, script, []string{r.key}, timeout).Int()
	if err != nil {
		return -1, err
	}
	return int32(val) / r.k, nil
}
