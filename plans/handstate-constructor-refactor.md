# HandState Constructor Refactoring Plan

## Problem Statement

The `internal/game/hand.go` file currently has 6 different constructors for `HandState`, creating a confusing and hard-to-maintain API:

1. `NewHandState` - Basic constructor with uniform chips
2. `NewHandStateWithRNG` - Adds RNG injection
3. `NewHandStateWithChips` - Individual chip counts
4. `NewHandStateWithChipsAndRNG` - Combines chips and RNG
5. `NewHandStateWithChipsAndDeck` - Chips with specific deck
6. `NewHandStateWithDeck` - Uniform chips with specific deck

### Issues
- **Combinatorial explosion**: Each feature combination needs a new constructor
- **Duplicated logic**: Initialization code repeated across constructors
- **Poor discoverability**: Developers must hunt through 6 constructors
- **Inconsistent implementation**: Some delegate to helpers, others duplicate
- **Hard to extend**: Adding features requires more constructor variants

## Proposed Solution: Option Pattern

Replace all constructors with a single `NewHandState` function that accepts variadic options.

### Core Design

```go
// Single public constructor
func NewHandState(playerNames []string, button int, smallBlind, bigBlind int, opts ...HandOption) *HandState

// Option type
type HandOption func(*handConfig)

// Private config struct
type handConfig struct {
    playerNames []string
    button      int
    smallBlind  int
    bigBlind    int
    chipCounts  []int       // Individual chips per player
    startChips  int         // Uniform starting chips (default: 1000)
    rng         *rand.Rand  // RNG for deck
    deck        *poker.Deck // Pre-shuffled deck (overrides RNG)
}
```

### Provided Options

#### Core Options
```go
// Set uniform chips for all players
WithUniformChips(chips int) HandOption

// Set individual chip counts
WithChips(chipCounts []int) HandOption

// Inject RNG for deterministic testing
WithRNG(rng *rand.Rand) HandOption

// Use pre-shuffled deck (overrides RNG)
WithDeck(deck *poker.Deck) HandOption

// Convenience for deterministic testing
WithSeed(seed int64) HandOption
```

#### Future Options (if needed)
```go
// Direct player injection (advanced)
WithPlayers(players []*Player) HandOption

// Override blind structure
WithBlinds(small, big int) HandOption

// Start from specific street (testing)
WithStartingStreet(street Street) HandOption

// Skip dealing (for hand replay)
WithoutDealing() HandOption
```

## Implementation Plan

### Phase 1: Add New API (Non-Breaking)
1. Add `HandOption` type and `handConfig` struct
2. Implement new `NewHandState` with options
3. Implement initial option functions
4. Add comprehensive tests for new API
5. Update documentation

### Phase 2: Migration
1. Mark old constructors as deprecated with clear migration guides
2. Update internal tests to use new API
3. Update examples and documentation
4. Create migration script for external users (if needed)

### Phase 3: Cleanup (Breaking Change)
1. Remove deprecated constructors
2. Remove `newHandStateWithPlayersAndDeck` helper
3. Update any remaining references
4. Bump minor version to indicate breaking change

## Usage Examples

### Before (Current)
```go
// Simple game
h := NewHandState(players, 0, 5, 10, 1000)

// With RNG
rng := rand.New(rand.NewSource(42))
h := NewHandStateWithRNG(players, 0, 5, 10, 1000, rng)

// With individual chips
h := NewHandStateWithChips(players, chips, 0, 5, 10)

// With chips and RNG (getting complicated...)
h := NewHandStateWithChipsAndRNG(players, chips, 0, 5, 10, rng)
```

### After (Proposed)
```go
// Simple game (defaults to 1000 chips)
h := NewHandState(players, 0, 5, 10)

// With RNG
h := NewHandState(players, 0, 5, 10, WithRNG(rng))

// With individual chips
h := NewHandState(players, 0, 5, 10, WithChips(chips))

// Combining options (intuitive!)
h := NewHandState(players, 0, 5, 10,
    WithChips(chips),
    WithSeed(42))

// Clear intent
h := NewHandState(players, 0, 5, 10,
    WithUniformChips(500),
    WithDeck(customDeck))
```

## Benefits

1. **Simplicity**: One function to learn instead of 6
2. **Extensibility**: New features don't require new constructors
3. **Readability**: Options clearly express intent
4. **Flexibility**: Any combination of options works
5. **Backwards Compatible Migration**: Can coexist with old API initially
6. **Type Safety**: Compile-time checking of options
7. **Good Defaults**: Most users need minimal configuration

## Testing Strategy

### Unit Tests
- Each option in isolation
- Option combinations
- Edge cases (nil checks, validation)
- Default behavior without options

### Integration Tests
- Ensure same behavior as old constructors
- Performance comparison (should be negligible)
- Concurrent usage safety

### Migration Tests
- Side-by-side comparison of old vs new
- Automated verification of equivalence

## Performance Considerations

The option pattern has minimal overhead:
- Options are applied once at construction
- No runtime penalty after construction
- Small allocation for config struct (stack-allocated in practice)
- Inlining likely for simple options

## Error Handling

Current constructors use `panic` for validation errors. The new API will:
1. Continue using `panic` for programming errors (mismatched array lengths)
2. Consider returning `(*HandState, error)` in future major version
3. Document all panics clearly in godoc

## Documentation Updates

1. Package documentation explaining the option pattern
2. Godoc examples for common scenarios
3. Migration guide from old to new API
4. README updates if constructor is shown there

## Success Criteria

- [ ] All existing tests pass with new implementation
- [ ] New API handles all current use cases
- [ ] Documentation clearly explains options
- [ ] Performance impact < 1% (measured via benchmarks)
- [ ] Clean migration path documented
- [ ] No breaking changes in Phase 1

## Timeline Estimate

- Phase 1: 2-3 hours (implementation + tests)
- Phase 2: 1-2 hours (migration + documentation)
- Phase 3: 30 minutes (cleanup after deprecation period)

## Alternative Considered: Builder Pattern

```go
h := NewHandBuilder().
    WithPlayers(players).
    WithButton(0).
    WithBlinds(5, 10).
    WithChips(chips).
    WithRNG(rng).
    Build()
```

Rejected because:
- More verbose than options
- Requires maintaining builder state
- Error handling more complex
- Less idiomatic in Go

## References

- [Uber Go Style Guide - Functional Options](https://github.com/uber-go/guide/blob/master/style.md#functional-options)
- [Dave Cheney - Functional options for friendly APIs](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis)
- Rob Pike's original [self-referential functions pattern](https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html)