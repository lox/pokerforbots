# Perfect Hash Evaluator Optimization Journey

## Overview

This document chronicles the optimization journey for the 7-card poker hand evaluator in `internal/evaluator/`. The goal was to achieve single-digit nanosecond evaluation times for Monte Carlo simulations.

## Performance Timeline

| Implementation | Performance | Notes |
|----------------|-------------|-------|
| Basic evaluator | ~72ns | Original implementation using basic logic |
| Map-based Perfect Hash | **~35ns** | **Current winner** - Go's built-in map optimization |
| Binary search arrays | ~105ns | Cache misses hurt performance |
| Bucketed linear scan | ~58ns | Overhead from bucket computation and loops |

## Approaches Tested

### 1. Map-Based Perfect Hash (~35ns) ✅

**Implementation**: Simple `map[int]HandRank` lookup using prime product as key.

```go
if v, ok := unsuitedTable[primeProd]; ok {
    return v
}
```

**Why it won**:
- Go's built-in map implementation is highly optimized
- Excellent hash functions with hardware acceleration
- Cache-friendly memory layout
- Branch prediction optimization
- No overhead from manual bucket computation

### 2. Bucketed Linear Scan (~58ns) ❌

**Implementation**: 1024 buckets with linear scan within each bucket.

```go
bucket := (primeProd ^ (primeProd >> 8) ^ (primeProd >> 16)) & 1023
start := bucketOffsets[bucket]
length := bucketLengths[bucket]
for i := 0; i < length; i++ {
    if unsuitedKeys[start+i] == primeProd {
        return unsuitedValues[start+i]
    }
}
```

**Why it failed**:
- Hash computation overhead for bucket selection
- Array indexing with bucket offsets adds latency
- Loop iteration overhead even for small buckets
- Cache pressure from multiple parallel arrays
- Manual optimization couldn't beat Go's built-in map

### 3. Binary Search Parallel Arrays (~105ns) ❌

**Implementation**: Sorted keys array with binary search.

**Why it failed**:
- Cache misses during binary search jumps
- Branch misprediction during search
- Multiple memory accesses for each comparison

## Key Learnings

### 1. Trust Go's Built-in Optimizations

Go's `map` implementation benefits from:
- Years of optimization work by the Go team
- Hardware-specific optimizations (CPU cache, branch prediction)
- SIMD instructions on modern processors
- Sophisticated hash functions (AES-based on ARM64)

### 2. Premature Optimization Anti-Pattern

We fell into the classic trap of assuming "simpler" data structures would be faster:
- Linear arrays "should" beat hash tables
- Manual bucket management "should" reduce overhead
- Reality: Modern hash table implementations are extremely sophisticated

### 3. Measurement Over Theory

Benchmarks revealed surprising results:
- Manual optimizations often hurt performance
- Cache effects dominate at this scale
- Compiler optimizations can be hard to predict

## Future Directions

### Minimal Perfect Hashing (MPH)

The next step for ultimate performance (targeting <10ns) would be implementing Minimal Perfect Hashing using external libraries:

- **CHD (Compress, Hash, and Displace)**: Minimal perfect hash with O(1) lookup
- **BBHash**: Fast construction, good for large datasets
- **Expected performance**: 5-8ns per evaluation

Trade-offs:
- Added dependency on external MPH library
- Increased complexity in build process
- Marginal gains for 4-5x development effort

### Current Recommendation

**Stick with map-based approach** (~35ns):
- 2x improvement over basic evaluator
- Zero external dependencies
- Maintainable and readable code
- Already fast enough for most Monte Carlo simulations

## Benchmark Commands

```bash
# Run performance benchmarks
cd internal/evaluator
go test -bench=BenchmarkEvaluate7_RandomHands -count=3

# Profile memory and CPU
go test -bench=BenchmarkEvaluate7_RandomHands -cpuprofile=cpu.prof
go test -bench=BenchmarkEvaluate7_RandomHands -memprofile=mem.prof
```

## Code Location

- Generator: `internal/evaluator/gen_perfect_hash.go`
- Runtime: `internal/evaluator/perfecthash.go`
- Tables: `internal/evaluator/ph_tables.go` (generated)
- Main entry: `internal/evaluator/evaluator.go`

## Conclusion

This optimization journey reinforced the importance of measurement-driven development and trusting well-optimized standard library implementations. The map-based perfect hash provides excellent performance gains while maintaining code simplicity and zero external dependencies.
