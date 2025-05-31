# No-Limit Hold’em Cash Game – 6-Max $1/$2
Full Rule Specification

---

## 1. Table Positions and Dealer Button

### 1.1 Dealer Button (“Button”)
- Physical marker that rotates **one seat clockwise** after every completed hand.
- Grants last action on every post-flop street.
- Determines blind placements.

### 1.2 Positional Map (6 Seats)
| Seat Index (relative to Button) | Common Name     | Mandatory Bet? | Pre-Flop Action Order |
|---------------------------------|-----------------|----------------|-----------------------|
| 0                                | Button (BTN)    | No             | 5th (last)            |
| +1                               | Small Blind (SB)| $1             | 1st                   |
| +2                               | Big Blind (BB)  | $2             | 2nd                   |
| +3                               | UTG             | —              | 3rd                   |
| +4                               | LJ              | —              | 4th                   |
| +5                               | HJ              | —              | 5th (BTN)             |

> With 5 or fewer players, keep SB and BB left of the Button; eliminate names beyond BB as seats disappear.

### 1.3 Heads-Up Adjustment
| Role | Blind | Pre-Flop Order | Post-Flop Order |
|------|-------|---------------|-----------------|
| Button | SB | **First** | **Last** |
| Opponent | BB | Second (last) | First |

---

## 2. Blinds and Dealing

1. **SB posts \$1**, **BB posts \$2**.
2. If a blind poster lacks sufficient chips, post all remaining chips and declare **all-in**.
3. Deal two hole cards **one at a time clockwise**, beginning with the SB.

---

## 3. Betting Structure

### 3.1 Streets
| Street | New Community Cards | First To Act |
|--------|--------------------|--------------|
| Pre-Flop | — | UTG (first active seat left of BB) |
| Flop | 3 | First active seat left of BTN |
| Turn | +1 (4th) | Same as Flop |
| River | +1 (5th) | Same as Flop |

A betting round ends when:
- All active players have contributed **identical amounts** for that street, **or**
- All but one player have folded.

### 3.2 Opening & Minimums
- **Minimum opening bet** on any street = current BB (\$2).
- **Minimum raise** = previous bet/raise size added to call amount.
  - Example: Bet \$10 → minimum reraise brings total to \$20 (call \$10 + raise \$10).
- **No cap** on raise count or size other than player stack (No Limit).

---

## 4. Player Actions

| Action | Preconditions | Result |
|--------|---------------|--------|
| Fold   | Anytime facing action | Hand dead; chips already in pot remain. |
| Check  | No outstanding bet | Pass action with no chips. |
| Bet    | No outstanding bet | Wager ≥ \$2. |
| Call   | Facing bet | Match bet; all-in allowed if stack < call. |
| Raise  | Facing bet | Call amount + minimum raise increment (or more, up to all-in). |
| All-In | Any turn | Commit entire stack; if < legal min-raise it **does not reopen** betting to prior actors. |

Action proceeds clockwise until betting closes by the rules in §3.1.

---

## 5. All-In Logic and Side Pots

1. Track each player’s **effective stack** (chips behind) when they act.
2. When one or more players go all-in and others continue betting:
   - Form a **main pot** equal to the smallest all-in amount × number of active players.
   - Excess contributions create **side pots** in ascending order of all-in sizes.
3. Only players who contributed to a given pot can win that pot.
4. Once every active player is all-in or betting completes normally, deal out any remaining community cards with no further betting.

---

## 6. Showdown Procedure

1. **Reveal order:** Last aggressor on the final street shows first; if no bet, SB (or first seat left of BTN) must show first.
2. Evaluate each hand’s **best five-card combination** from hole + board.
3. **Award pots sequentially**: main pot, then side pots from earliest to latest creation (order does not affect result).
4. For each pot:
   - Highest ranked hand among eligible players wins.
   - **Identical best hands** split that pot (see §7).

If all but one player folded earlier, skip showdown; sole survivor wins all pots.

---

## 7. Split & Chopped Pots

- If two or more eligible players hold exactly equal five-card hands, divide that pot **equally**.
- **Odd chip** (if total is not divisible) goes to the player **closest clockwise to the Button** among those tying.
- Apply split logic separately to every side pot.

---

## 8. Hand Completion and Rotation

1. After pot distribution, push chips; muck all cards.
2. **Advance Button one seat clockwise**.
3. New SB and BB post blinds; begin next hand at §2.

---

## 9. Edge-Case Clarifications

- **Straddles, antes, dead blinds, time controls, and table etiquette** are *out of scope*; this file governs only core gameplay mechanics.
- **Deck misdeals, exposed cards, or run-it-twice variants** require separate handling logic and are not covered here.
- For pot division, always perform **integer chip accounting** to avoid rounding drift in long simulations.

---

_This document is designed to be machine-readable and unambiguous for enforcement in a 6-max $1/$2 no-limit Hold’em bot._
