# Complex Bot Improvement Plan

## Current Performance (2024-09-24)

### Regression Test Results
- **Heads-up**: +16.02 BB/100 vs baseline (VPIP: 26.7%, PFR: 25.9%) ✅
- **vs NPCs**: +242.79 BB/100 (VPIP: 28.5%, PFR: 26.5%) ✅
- **Population (6-max)**: +0.72 BB/100 (VPIP: 14.3%, PFR: 12.2%) ⚠️

### Key Issues Identified
1. **Too tight in multi-way pots** - 14% VPIP in 6-max is way below optimal (should be 18-22%)
2. **No opponent tracking** - Doesn't adjust to opponent tendencies within hands
3. **Position miscategorization** - Treats 6-max positions like full-ring

## Improvements In Progress

### 1. Fix Multi-way VPIP (COMPLETED)
- **Problem**: Bot uses same ranges for 6-max as full-ring
- **Solution**: Adjusted position calculation to treat distance >= 2 as middle position in 6-max
- **Test Results**:
  - vs Calling Stations (6-max): VPIP increased to 32% ✅
  - Heads-up: +20.26 BB/100, VPIP: 28% (maintained strong performance) ✅
  - Population: VPIP ~14-20% depending on opponent tightness
- **Impact**: Bot now adjusts ranges properly for 6-max games

### 2. In-Hand Opponent Tracking (IN PROGRESS)
Since the server design is stateless with random opponent selection each hand, we can't track opponents across hands. However, we CAN track their actions within the current hand to narrow ranges.

#### Implementation Strategy
Track each opponent's actions within the hand:
- **Preflop**: Open, 3-bet, call, limp → narrow their range
- **Postflop**: Continuation bet, check-raise, sizing tells
- **Multi-street**: Aggression patterns, timing

#### Range Narrowing Examples
- EP opener → ~10% range (77+, AJo+, KQo, ATs+)
- 3-bettor → ~4% range (TT+, AQs+, AKo)
- Cold caller → Capped range (no QQ+/AK)
- Limper → Weak range (small pairs, suited connectors)

### 3. Dynamic Range Adjustments (TODO)

#### Based on Table Dynamics
- **Loose table** (3+ limpers) → Tighten up, value bet more
- **Tight table** (lots of folds) → Steal more, widen ranges
- **Aggressive table** (frequent 3-bets) → Polarize ranges

#### Based on Stack Sizes
- **Short stacks** (<40BB) → Push/fold ranges
- **Deep stacks** (>150BB) → More speculative hands
- **Tournament pressure** → ICM considerations

## Next Steps

### High Priority
- [ ] Implement opponent action tracking structure
- [ ] Add range narrowing logic based on preflop actions
- [ ] Adjust postflop aggression based on opponent's perceived range
- [ ] Test multi-way improvements with regression suite

### Medium Priority
- [ ] Add board texture classification (wet/dry, coordinated)
- [ ] Implement SPR-based commitment thresholds
- [ ] Improve bluffing frequencies based on board runouts
- [ ] Add blocker awareness for bluffs

### Low Priority
- [ ] Multi-street planning for bluffs
- [ ] Reverse implied odds calculations
- [ ] Meta-game adjustments (balance vs exploitation)

## Testing Plan

1. **Quick validation** - Run heads-up regression to ensure no regression
2. **Population test** - Verify VPIP increases to 18-22% range
3. **NPC benchmark** - Maintain or improve +240 BB/100 performance
4. **Extended test** - 50k hands to validate statistical significance

## Code Changes Log

### 2024-09-24
- **Fixed NPC identification bug** in regression tester (was showing 50%+ VPIP incorrectly)
- **Adjusted position calculation** for 6-max games (treat distance >= 2 as middle, not early)
- Started implementing in-hand opponent tracking framework

## Performance Targets

### Short Term (This Session)
- Population VPIP: 14% → 20%
- Population BB/100: +0.72 → +5.00
- Maintain heads-up and NPC performance

### Medium Term (Next Session)
- Implement full opponent tracking
- Achieve +10 BB/100 in population games
- Reduce variance through better decision making

### Long Term
- Beat all NPC types by 15+ BB/100
- Achieve consistent profits in mixed population games
- Optimize for tournament/survival scenarios

## Notes

### Design Constraints
Per the server design (`docs/design.md`):
- Each hand is completely independent
- Random bot selection from pool
- No persistent state between hands
- No way to identify specific opponents
- All tracking must be within single hand

### Key Insights
- Can't do traditional HUD stats or long-term opponent modeling
- Must focus on in-hand dynamics and immediate adjustments
- Position and stack sizes are the main persistent factors
- Board texture and betting patterns are key for in-hand adjustments