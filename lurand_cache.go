package lurand

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	script = `
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
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

		redis.call("SET", key .. idx, rep, "EX", ttl)
		redis.call("SET", key, max - 1, "EX", ttl)
		redis.call("DEL", key .. max)

		return val
	`

	ONE_DAY = 24 * 60 * 60
)

type CacheLUR struct {
	*redis.Client

	key string
	k   int32
	ttl int32
}

func NewCacheLUR(ctx context.Context, client *redis.Client, key string, ttl int32) *CacheLUR {
	return NewCacheLUR__(ctx, client, key, ONE_MILLION, 1, ttl)
}

func NewCacheLUR_(ctx context.Context, client *redis.Client, key string, max, ttl int32) *CacheLUR {
	return NewCacheLUR__(ctx, client, key, max, 1, ttl)
}

func NewCacheLUR__(ctx context.Context, client *redis.Client, key string, max, k, ttl int32) *CacheLUR {
	if max <= 0 {
		panic("Invalid max setting")
	}
	if k <= 0 {
		panic("Invalid k setting")
	}
	if client == nil {
		panic("Invalid client setting")
	}
	ok, err := client.SetNX(ctx, key, max*k, time.Duration(ttl)*time.Second).Result()
	if err != nil {
		panic(err)
	}
	if !ok {
		panic("Key already exist")
	}
	return &CacheLUR{
		Client: client,
		key:    key,
		k:      k,
		ttl:    ttl,
	}
}

func (r *CacheLUR) Int31n(ctx context.Context) (int32, error) {
	val, err := r.Eval(ctx, script, []string{r.key}, r.ttl).Int()
	if err != nil {
		return -1, err
	}
	return int32(val) / r.k, nil
}
