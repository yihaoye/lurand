# LuRand
Efficient Large Scale Unique Random Number Generator (Concurrent Safety).  

## Install
`go get github.com/yihaoye/lurand`  

## Usage Example without Cache
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
    r3 := lurand.New__(12, 3)
    for i := 0; i < 12; i++ {
        num := r3.Int31n() // outputs: 0, 3, 2, 0, 3, 1, 1, 2, 2, 0, 1, 3
    }
}
```

## Usage Example with Cache
For example, in system design further usage, if need to scale up further (current lib limit within 100 million since it cost ~4GB), it is better to implement with cache db (e.g. Redis) or its cluster instead of memory to store LUR.mapping and LUR.max (easy to scale up to TB level), which also ensure availability, and could apply Lua script to promise system level concurrent safety.  
```go
package main

import (
    github.com/yihaoye/lurand
)

func main() {
    InitCache_("localhost:6379", 60)
	ctx := context.Background()
    r1 := NewCacheLUR_(ctx, "{prefix_key}", 10_000)
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
And then test with [Code Example](./lurand_cache_test.go)  
