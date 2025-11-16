# Hand History Recording (PHH)

PokerForBots can persist every hand in the [Poker Hand History (PHH)](https://phh.readthedocs.io/en/stable/) format for post-game analysis, debugging, and dataset generation. PHH is a TOML-based format designed specifically for poker AI research, so it can be parsed with standard tooling (Go's `encoding/toml`, Python's `tomllib`, etc.).

## Enabling Recording

Hand history recording is disabled by default. Enable it either via `pokerforbots spawn` (embedded server) or the standalone `pokerforbots server` command.

### Spawn Command

```bash
pokerforbots spawn \
  --spec "complex:3,random:3" \
  --hand-history \
  --hand-history-dir ./hands \
  --hand-history-flush-secs 5 \
  --hand-history-flush-hands 50
```

### Standalone Server

```bash
pokerforbots server \
  --hand-history \
  --hand-history-dir /var/lib/pokerforbots/hands \
  --hand-history-flush-secs 10 \
  --hand-history-flush-hands 100 \
  --hand-history-hole-cards   # optional, includes all hole cards
```

Available flags:

| Flag | Description |
| --- | --- |
| `--hand-history` | Enable PHH recording |
| `--hand-history-dir` | Base directory for all games (default `hands`) |
| `--hand-history-flush-secs` | Interval for periodic flushes (default 10s) |
| `--hand-history-flush-hands` | Flush after N hands even if timer hasn't fired (default 100) |
| `--hand-history-hole-cards` | Include every player's hole cards (default masks as `????`) |

## File Layout

Each game writes to its own directory under the base `hands/` folder:

```
hands/
└── game-default/
    └── session.phhs
```

Flushes append to the session file, so you can tail the file in near real time or process it after a long run. By default, hole cards are masked to avoid leaking private information; enable `--hand-history-hole-cards` only when debugging in a trusted environment.

## Sample Entry

```
[1]
variant = "NT"
table = "default"
seat_count = 3
seats = [2, 3, 1]
antes = [0, 0, 0]
blinds_or_straddles = [5, 10, 0]
min_bet = 10
starting_stacks = [1000, 1000, 1000]
finishing_stacks = [1000, 990, 1010]
winnings = [0, 0, 10]
actions = [
  "d dh p1 ????",
  "d dh p2 ????",
  "d dh p3 ????",
  "p1 cbr 10",
  "p2 cc",
  "p3 f",
  "p1 sm AhAd",
]
players = ["bob", "charlie", "alice"]
hand = "hand-00042"
time = "12:34:56"
time_zone = "Local"
time_zone_abbreviation = "AEDT"
day = 14
month = 11
year = 2025
```

All player-indexed arrays (`players`, `seats`, `antes`, `blinds_or_straddles`, `starting_stacks`, `finishing_stacks`, and `winnings`) start with the small blind and wrap clockwise. That means the button is implicitly the last entry in multi-handed games, while in heads-up play the button/small blind naturally occupies the first slot.

Fields listed in the [required](https://phh.readthedocs.io/en/stable/required.html) and [optional](https://phh.readthedocs.io/en/stable/optional.html) PHH spec are emitted verbatim; we no longer emit `_`-prefixed extras.

## Parsing PHH Files

PHH files are standard TOML and can be parsed with any TOML library:

**Python:** Use the built-in `tomllib` (Python 3.11+) or the [pokerkit](https://github.com/uoftcprg/pokerkit) library which includes comprehensive PHH support and hand evaluation tools.

**Go:** Use `github.com/BurntSushi/toml` to decode into structs.

**Other languages:** Most languages have standard TOML parsers that work with PHH.

## Rendering in the Pretty Format

You can replay a PHH file using the same ANSI-rendered output as `pokerforbots spawn --output hand-history`:

```bash
pokerforbots hand-history render hands/game-default/session.phhs

# Only render the first 3 hands
pokerforbots hand-history render hands/game-default/session.phhs --limit 3
```

The command reuses the pretty-print monitor, so you'll see the familiar `*** HOLE CARDS ***`, flop/turn/river headers, and winner summaries directly from your saved session.
