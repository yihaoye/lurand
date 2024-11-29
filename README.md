# LuRand
Efficient Large Scale Unique Random Number Generator (Concurrent Safety).  

## Install
`go get github.com/yihaoye/lurand`  

## Usage Example
```go
package main

import (
    github.com/yihaoye/lurand
)

func main() {
    // [0..1_000_000)
    r1 := lurand.New()
    num := r1.Intn() // e.g. 876783

    // [0..{custom})
    r2 := lurand.New_(10)
    for i := 0; i < 10; i++ {
        num := r2.Intn() // outputs: 4, 8, 3, 0, 9, 1, 6, 2, 5, 7
    }
    r2.Intn() // panic: No more numbers available

    // Also support New32() New64()
    // And Int31n() Int63n()
}
```

## Further Usage
For example, in system design, if need to scale up further (current lib limit within 100 million since it cost ~4GB), it is better to implement with cache db (e.g. Redis) or its cluster instead of memory to store LUR.mapping and LUR.max (easy to scale up to TB), which also ensure availability, and could apply Lua script to promise system level concurrent safety.  
