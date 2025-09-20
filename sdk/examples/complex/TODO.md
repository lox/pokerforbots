# TODO: Make the complex bot a winner vs NPCs (Refined)

This plan upgrades the "complex" bot into a consistent winner versus the built‑in NPC bots by tightening preflop ranges, adding fold discipline, using standardized bet sizing, adding simple postflop hand/draw heuristics, exploiting opponent tendencies, and applying SPR awareness — all localized to `sdk/examples/complex/main.go`.

## Current Status
- SDK plumbing fixed: chip payouts applied on hand_result; `StartingChips` tracked in state.
- Patch 1 implemented: preflop ranges/sizing, fold thresholds, SPR guards, standardized bet sizes.
- Patch 2 foundation implemented: coarse postflop `classifyPostflop()` integrated into decisions; min-raise sizing respected (uses `MinRaise` and `MinBet`).
- Validation (3k hands): Net +5228.5 BB (+174.28 BB/100), Win rate 2.2% (67 wins), Showdown WR 28.7%, CO/Button positive; MP slightly negative.
- Batch (5×10k, no infinite bankroll): per-run BB/100 ≈ [63.3, 45.5, 37.6, 38.9, 56.8], mean ≈ +48.4 BB/100. CO still slightly negative; non-showdown slightly negative (expected pre-Patch 2 refinement).

## Remaining Issues
- Postflop strength still heuristic; lacks hand/draw classification and board texture handling.
- Opponent profiling not yet used to adapt frequencies/sizes.

## Success Criteria
- Primary: BB/100 > 0 over 50k hands vs default 3‑NPC mix (target +20 to +80 BB/100).
- Showdown Win Rate ~55% with substantially lower showdown frequency (<25%).
- BTN/CO clearly positive; early positions tighter and near breakeven.
- 95% CI lower bound on BB/100 after 5×10k runs > −20.

## Patch 1 (ship first): Stop-the-bleeding TA style
Minimal edits with numeric rules; biggest win quickly.

### A) Preflop tighten & fixed sizing
- Open sizes:
  - Standard open = min(2.5 bb, 3×BB); use 3 bb if ≥2 limpers.
- 3‑bet sizes:
  - In Position (IP) = 8.5 bb, Out of Position (OOP) = 10 bb.
- 4‑bet size:
  - Flat 22 bb; shove if effective stack ≤ 35 bb.

Open ranges (6‑max):
- UTG/LJ (≈13.5%): 77+, AJo+, KQo; suited A5s+, KTs+, QTs+, JTs, T9s.
- CO (≈23%): 55+, A9s+, AJo+, KJo+, K9s+, QTs+, JTs, T9s, 98s.
- BTN (≈46%): 22+, any Ax, K8s+, Q9s+, J9s+, T8s+, 65s+, KTo+, QTo+, JTo.
- SB: ~35% open vs folds; vs completes/limpers, raise most playable (value‑heavy).
- BB defend vs min‑raise: 22+, Kx, Q8s+, J8s+, any suited connectors 54s+, suited gappers 64s+, offsuit broadways.

Facing raises:
- 3‑bet VALUE (all seats): TT+, AQs+, AKo.
- 3‑bet BLUFF (BTN/CO only): A5s–A2s, K9s, QTs.
- 4‑bet: QQ+/AK; otherwise fold.

### B) Fold discipline (equity thresholds)
Implement `shouldFold(req, equity)` using pot‑fraction thresholds:

Street | Facing bet size        | Minimum equity to continue
------ | ----------------------- | ---------------------------
Flop   | ≤ 33% pot               | > 0.25 OR 8 outs
       | 34–66% pot              | > 0.40 OR pair+draw (≈10 outs)
       | > 66% pot               | > 0.55 (TPTK+ or strong draw)
Turn   | ≤ 50% pot               | > 0.35
       | 51–99% pot              | > 0.55
       | Jam                     | ≥ 0.65 only
River  | ≤ 40% pot               | > 0.45
       | > 40% pot               | > 0.60

Fallback: if equity unknown or placeholder < 0.25, auto‑fold to > ½‑pot.

### C) Bet sizing helpers
Add `betSize(req, pct)` that clamps to `[req.MinBet, b.state.Chips]` from `pct * req.Pot`.

Scenario                          | Size (% pot)
--------------------------------- | -----------
Flop c‑bet HU dry board           | 0.33
Flop c‑bet HU wet / multi‑way     | 0.50 (skip in 3‑way+ if no equity)
Turn value / semi‑bluff           | 0.50
River value                       | 0.75 if range‑adv; else 0.60
Pure bluff (rare, late position)  | 0.33

### D) SPR awareness (lightweight)
- If `SPR < 2` and `equity > 0.60` → OK to stack off.
- If `SPR > 8` and `equity < 0.55` → pot‑control: check or call small; never raise.

Acceptance for Patch 1
- Showdown rate ≤ 35%.
- BB/100 ≥ −50 over an initial 10k‑hand test.

Status: initial 3k-hand run achieved +174.3 BB/100 with Showdown WR 28.7% (frequency ~5.7%). Needs more samples (≥5×10k) to confirm trend.

## Patch 2: Postflop hand/draw classifier (coarse buckets)
Implement `classifyPostflop()` returning enum + equity bucket; adjust −0.05 per extra villain beyond HU.

Class                 | Equity (HU)
--------------------- | -----------
Overpair/Trips+       | 0.80
Top Pair Top Kicker   | 0.65
Top Pair Weak Kicker  | 0.55
Second Pair           | 0.42
8+‑out draw           | 0.40
4–7‑out draw          | 0.25
Air                   | 0.10

Use bucket equity in `shouldFold()` and bet‑size selection.

Acceptance for Patch 2
- Over 20k hands: BB/100 ≥ +10; Showdown Win Rate ≥ 55%; c‑bet success (win w/o showdown) ≥ 45% in HU single‑raised pots.

## Patch 3: Opponent exploitation (VPIP & AF)
Compute tags every 50 hands (per seat):

Tag    | Rule                          | Adjustments
------ | ----------------------------- | ----------------------------------------------
Station| VPIP > 45% and AF < 1         | −50% bluff freq; value bet ≥ 0.60 pot with ≥0.55 equity
Aggro  | AF ≥ 2.5                      | Call down slightly wider (−0.05 equity), trap premiums (check‑raise 25%)
Tight  | VPIP < 18%                    | Steal wider on BTN (up to 60%); bluff 3‑bet 0%

Apply tag in HU pots via `deriveVillainTag(seat)`; in multiway, blend conservatively.

Acceptance for Patch 3
- +5 BB/100 vs calling‑station only sim; no regression vs mixed field.

## Patch 4 (optional): Decision metrics/logging
Log per decision: handNum, street, position, SPR, villainTag, equityBucket, action, size, reason.

## Implementation Notes (single file)
- Modify `sdk/examples/complex/main.go` only.
- Add helpers:
  - `betSize(req, pct float64) int`
  - `shouldFold(req protocol.ActionRequest, equity float64) bool`
  - `classifyPostflop() (class string, equity float64)`
  - `deriveVillainTag(seat int) string`
  - `calcSPR(req protocol.ActionRequest) float64`
- In `makeStrategicDecision`:
  - Route preflop → `preflopDecision(req)` with the ranges and sizes above.
  - Postflop → use `classifyPostflop`, `shouldFold`, sizing table, and SPR rules.
- Keep `evaluateHandStrength` as a thin wrapper over `classifyPostflop` for now.

## Validation Loop
- Run 5×10k hands with stats enabled:
  - `task server -- --infinite-bankroll --hands 10000 --timeout-ms 20 --npc-bots 3 --enable-stats --stats-depth=full` (per run)
  - `go run ./sdk/examples/complex`
- Pass if:
  - Mean BB/100 > 0 and 95% CI lower bound > −20.
  - Showdown frequency falls below 35% after Patch 1; to ≤25% after Patch 2.
- Scenario checks (single 20k hand runs):
  - 3 calling stations → should be clearly profitable (value heavy).
  - 3 aggro bots → around breakeven or better (fold discipline working).
  - 2 random + 1 tight → BTN steals show strong positive result.

## Benchmarks / Targets
- Short‑term: eliminate catastrophic leaks (BB/100 > −50), showdown rate < 40%.
- Mid‑term: BB/100 > 0 in 50k hands; BTN/CO strongly positive; showdown ≤ 25%.
- Long‑term: BB/100 +20 to +80 vs default NPC mix; stable across seeds.

## Nice‑to‑Have (later)
- Board texture classifier (A‑high dry vs low/connected/wet) to refine c‑betting.
- Mixed frequencies by RNG for balance.
- Simple range heatmaps for self‑checks in logs.
