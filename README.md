# lurand
Efficient Large Unique Random Generator (Concurrent Safety).  

## Usage Example
```go
// [0..1_000_000]
rg1 := New()
num := rg1.Intn()

// [0..{custom}]
rg2 := New_(10)
for i := 0; i < 10; i++ {
    num := rg2.Intn() // outputs: 4, 8, 3, 0, 9, 1, 6, 2, 5, 7
}
rg2.Intn() // panic: No more numbers available
```
