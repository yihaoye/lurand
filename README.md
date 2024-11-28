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
For example, in system design, if need to scale up further, could use multiple servers (like 100), each sever has an unique number from 0 to 99, when request come in, use load balance to randomly pick a server to generate the number with the library, and then concate the final result as `server_num.append(lib_num)`, and offline server if corresponding numbers are used up.  
If worry about server availability, it is better to implement with db or cache instead of memory to store LUR.mapping and LUR.max.  
