# LuRand
Efficient Large Scale Unique Random Number Generator (Concurrent Safety).  

## Install
`go get github.com/yihaoye/lurand`  

## Usage Example without CacheDB
```go
package main

import (
    github.com/yihaoye/lurand
)

func main() {
    // [0..1_000_000)
    r1 := lurand.New()
    num := r1.Int31n() // e.g. 876783

    // [0..{custom})
    r2 := lurand.New_(10)
    for i := 0; i < 10; i++ {
        num := r2.Int31n() // outputs: 4, 8, 3, 0, 9, 1, 6, 2, 5, 7
    }
    r2.Int31n() // panic: No more numbers available

    // Also support max duplicate times (i.e. k, default is 1) for each random number
    r3 := lurand.New__(4, 3)
    for i := 0; i < 4; i++ {
        num := r3.Int31n() // outputs: 0, 3, 2, 0, 3, 1, 1, 2, 2, 0, 1, 3
    }
}
```

## Usage Example with CacheDB
For example, in system design further usage, if need to scale up further (current lib limit within 100 million since it cost ~4GB), it is better to implement with cache db (e.g. Redis) or its cluster instead of memory to store LUR.mapping and LUR.max (easy to scale up to TB level), which also ensure availability, and could apply Lua script to promise system level concurrent safety.  
```go
package main

import (
    github.com/yihaoye/lurand
)

func main() {
    client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		PoolSize: 10,
	})
    ctx := context.Background()
    r1 := lurand.NewCacheLUR_(ctx, client, "{prefix_key}", 1_000_000_000, 0) // setup max range depends on Redis (cluster) memory capacity, last param 0 here means no ttl (specify ttl based on second)
    num, err := r1.Int31n(ctx)
    // ...
}
```

### Test
Setup Redis with docker:  
```bash
docker pull redis:latest

docker run -d --name my-redis -p 6379:6379 redis:latest
# mkdir -p ~/redis-data
# docker run -d --name my-redis -p 6379:6379 -v ~/redis-data:/data redis:latest --save 60 1
```  
And then run [test code example](./lurand_cache_test.go)  

## Further Usage
To scale up further, could use multiple servers (for example 100), each sever has an unique id from 0 to 99, when request come in, use load balance to randomly pick one server to generate the number with the library, and then concate the final result as `server_num.append(lib_num)`, and offline server if corresponding numbers are used up.  
Option 2: although the unique random number concatenation of two instances cannot guarantee statistical randomness, it can still achieve safe unpredictable unique value generation. The first one generates a constant prefix, and then the second one continues to generate until it is exhausted or the randomness ends, and the former generates a new prefix again. The disadvantage is that it may be wasteful in some cases.  
