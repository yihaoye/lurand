# lurand
Efficient Large Unique Random Generator (Concurrent Safety).  

## Usage Example
```go
// [0..1_000_000]
rng := New()
num := rng.Intn()

// [0..{custom}]
rng := New_(1_500_000)
num := rng.Intn()
```
