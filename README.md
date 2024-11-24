# lurand
Efficient Large Unique Random Generator (Concurrent Safety).  

## Usage Example
```go
// [0..1_000_000)
rg1 := New()
num := rg1.Intn() // e.g. 876783

// [0..{custom})
rg2 := New_(10)
for i := 0; i < 10; i++ {
    num := rg2.Intn() // outputs: 4, 8, 3, 0, 9, 1, 6, 2, 5, 7
}
rg2.Intn() // panic: No more numbers available
```

## Further Usage
For example, in system design, if we would like to scale up further, we could use multiple servers (like 100), each sever has an unique number from 0 to 99, when request come in, use load balance to randomly pick a server to generate the number with the library, and then concate the final result as `server_num.append(lib_num)`, and offline server if corresponding numbers are used up.  
If worry about server availability, it is better to implement with db instead of memory for LUR.mapping and LUR.max.  
