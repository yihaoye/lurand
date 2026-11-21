package lurand

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// All state of one CacheLUR lives in a single Redis hash (KEYS[1]):
//
//	field "m"      -> current available range (max)
//	field "<idx>"  -> value currently held by virtual slot idx (absent = idx itself)
//
// Keeping everything in one key means:
//   - the whole state expires atomically (no partially expired mapping that would yield duplicates)
//   - keys of different generators can never collide (prefix "a" vs "a1")
//   - the script touches exactly the key it declares, so it is valid on Redis Cluster
//
// ARGV[1] = ttl in seconds (0 = no expiry), ARGV[2] = random integer in [0, 2^53) chosen by the client.
const (
	script = `
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local rnd = tonumber(ARGV[2])
		local max = tonumber(redis.call("HGET", key, "m"))
		if not max then
			return redis.error_reply("` + errKeyMissing + `")
		end
		if max <= 0 then
			return redis.error_reply("` + errNoMore + `")
		end

		-- math.fmod is exact for integers < 2^53 (Lua 5.1 "%" goes through a/b and may round)
		local idx = math.fmod(rnd, max)
		local last = max - 1
		local val = redis.call("HGET", key, idx)
		if not val then
			val = idx
		end
		if idx ~= last then
			local rep = redis.call("HGET", key, last)
			if not rep then
				rep = last
			end
			redis.call("HSET", key, idx, rep)
		end
		redis.call("HDEL", key, last)
		redis.call("HSET", key, "m", last)
		if ttl > 0 then
			redis.call("EXPIRE", key, ttl)
		end

		return tonumber(val)
	`

	initScript = `
		if redis.call("HSETNX", KEYS[1], "m", ARGV[1]) == 0 then
			return 0
		end
		if tonumber(ARGV[2]) > 0 then
			redis.call("EXPIRE", KEYS[1], ARGV[2])
		end
		return 1
	`

	errNoMore     = "No more numbers available"
	errKeyMissing = "LUR key not found (expired or never initialized)"

	ONE_DAY = 24 * 60 * 60

	maxRand53 = 1 << 53
)

type CacheLUR struct {
	*redis.Client

	key string
	k   int32
	ttl int32

	mu  sync.Mutex
	rnd *rand.Rand
}

func NewCacheLUR(ctx context.Context, client *redis.Client, key string, ttl int32) *CacheLUR {
	return NewCacheLUR__(ctx, client, key, ONE_MILLION, 1, ttl)
}

func NewCacheLUR_(ctx context.Context, client *redis.Client, key string, max, ttl int32) *CacheLUR {
	return NewCacheLUR__(ctx, client, key, max, 1, ttl)
}

// NewCacheLUR__ creates a new generator state under key. ttl is in seconds, 0 means no expiry;
// with ttl > 0 the expiry is refreshed on every Int31n call (sliding expiry of the whole state).
// It panics if the key already exists; use OpenCacheLUR to attach to an existing state.
func NewCacheLUR__(ctx context.Context, client *redis.Client, key string, max, k, ttl int32) *CacheLUR {
	if k <= 0 {
		panic("Invalid k setting")
	}
	if max <= 0 || int64(max)*int64(k) > math.MaxInt32 {
		panic("Invalid max setting")
	}
	if ttl < 0 {
		panic("Invalid ttl setting")
	}
	if client == nil {
		panic("Invalid client setting")
	}
	created, err := client.Eval(ctx, initScript, []string{key}, max*k, ttl).Int()
	if err != nil {
		panic(err)
	}
	if created == 0 {
		panic("Key already exist")
	}
	return newCacheLUR(client, key, k, ttl)
}

// OpenCacheLUR attaches to a state previously created by NewCacheLUR* (e.g. after a process restart).
// k must be the same value the state was created with.
func OpenCacheLUR(ctx context.Context, client *redis.Client, key string, k, ttl int32) (*CacheLUR, error) {
	if k <= 0 || ttl < 0 || client == nil {
		return nil, errors.New("invalid k, ttl or client setting")
	}
	n, err := client.Exists(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New(errKeyMissing)
	}
	return newCacheLUR(client, key, k, ttl), nil
}

func newCacheLUR(client *redis.Client, key string, k, ttl int32) *CacheLUR {
	return &CacheLUR{
		Client: client,
		key:    key,
		k:      k,
		ttl:    ttl,
		rnd:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *CacheLUR) Int31n(ctx context.Context) (int32, error) {
	r.mu.Lock()
	rnd := r.rnd.Int63n(maxRand53)
	r.mu.Unlock()

	val, err := r.Eval(ctx, script, []string{r.key}, r.ttl, rnd).Int64()
	if err != nil {
		return -1, err
	}
	return int32(val / int64(r.k)), nil
}
