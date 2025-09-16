package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/lox/pokerforbots/internal/game"
)

var (
	seed    = flag.Int64("seed", 0, "Random seed (0 for current time)")
	numBots = flag.Int("bots", 4, "Number of bots (2-6)")
	verbose = flag.Bool("v", false, "Verbose output (show all actions)")
)

type BotPlayer struct {
	Name      string
	Strategy  string
	Chips     int
	HoleCards game.Hand
}

func main() {
	flag.Parse()

	// Validate bot count
	if *numBots < 2 || *numBots > 6 {
		fmt.Println("Number of bots must be between 2 and 6")
		return
	}

	// Set seed
	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(*seed))

	fmt.Printf("=====================================\n")
	fmt.Printf("Single Hand Simulator\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("Seed: %d\n", *seed)
	fmt.Printf("Bots: %d\n", *numBots)
	fmt.Printf("=====================================\n\n")

	// Create bot players
	players := make([]BotPlayer, *numBots)
	playerNames := make([]string, *numBots)
	strategies := []string{"calling", "random", "aggressive"}

	for i := 0; i < *numBots; i++ {
		strategy := strategies[i%len(strategies)]
		players[i] = BotPlayer{
			Name:     fmt.Sprintf("Bot%d_%s", i, strategy),
			Strategy: strategy,
			Chips:    100, // Starting chips
		}
		playerNames[i] = players[i].Name
	}

	// Create hand state with seeded deck
	deck := game.NewDeckWithSeed(*seed)
	h := game.NewHandStateWithDeck(playerNames, 0, 5, 10, 100, deck)

	// Store hole cards for display
	for i := 0; i < *numBots; i++ {
		players[i].HoleCards = h.Players[i].HoleCards
	}

	// Print initial state
	fmt.Printf("HAND START\n")
	fmt.Printf("Button: Seat %d\n", h.Button)
	fmt.Printf("Blinds: 5/10\n\n")

	fmt.Printf("Players:\n")
	for i, p := range players {
		blindStr := ""
		if *numBots == 2 {
			if i == h.Button {
				blindStr = " (SB)"
			} else {
				blindStr = " (BB)"
			}
		} else {
			switch i {
			case (h.Button + 1) % *numBots:
				blindStr = " (SB)"
			case (h.Button + 2) % *numBots:
				blindStr = " (BB)"
			}
		}
		fmt.Printf("  Seat %d: %s (%d chips)%s\n", i, p.Name, p.Chips, blindStr)
		if *verbose {
			fmt.Printf("    Hole cards: %s\n", formatHand(p.HoleCards))
		}
	}
	fmt.Printf("\n")

	// Pass rng to action selection
	// Run the hand
	streetNames := map[game.Street]string{
		game.Preflop:  "PREFLOP",
		game.Flop:     "FLOP",
		game.Turn:     "TURN",
		game.River:    "RIVER",
		game.Showdown: "SHOWDOWN",
	}

	lastStreet := h.Street
	actionCount := 0

	for !h.IsComplete() && actionCount < 100 { // Safety limit
		// Check for new street
		if h.Street != lastStreet {
			fmt.Printf("\n=== %s ===\n", streetNames[h.Street])
			if h.Street > game.Preflop && h.Street < game.Showdown {
				fmt.Printf("Board: %s\n", formatHand(h.Board))
				fmt.Printf("Pot: %d\n\n", h.Pots[0].Amount)
			}
			lastStreet = h.Street
		}

		// Get current player
		if h.ActivePlayer < 0 || h.ActivePlayer >= len(h.Players) {
			// No active players, advance to showdown
			if h.Street != game.Showdown {
				h.NextStreet()
				continue
			}
			break
		}

		activePlayer := h.Players[h.ActivePlayer]
		botPlayer := players[h.ActivePlayer]

		// Get valid actions
		validActions := h.GetValidActions()
		if len(validActions) == 0 {
			break
		}

		// Select action based on strategy
		action, amount := selectAction(botPlayer.Strategy, validActions, h.Pots[0].Amount,
			h.CurrentBet-activePlayer.Bet, activePlayer.Chips, rng)

		// Process action
		err := h.ProcessAction(action, amount)
		if err != nil {
			fmt.Printf("Error processing action: %v\n", err)
			break
		}

		// Print action
		actionStr := formatAction(action, amount, h.CurrentBet-activePlayer.Bet)
		fmt.Printf("Seat %d (%s): %s - Pot: %d\n",
			h.ActivePlayer, botPlayer.Name, actionStr, h.Pots[0].Amount)

		actionCount++
	}

	// Show showdown if reached
	if h.Street == game.Showdown || h.IsComplete() {
		fmt.Printf("\n=== SHOWDOWN ===\n")
		fmt.Printf("Final Board: %s\n", formatHand(h.Board))
		fmt.Printf("Final Pot: %d\n\n", h.Pots[0].Amount)

		// Show hands of non-folded players
		for i, p := range h.Players {
			if !p.Folded {
				fmt.Printf("Seat %d (%s): %s\n", i, players[i].Name, formatHand(players[i].HoleCards))
			}
		}

		// Determine and show winners
		winners := h.GetWinners()
		fmt.Printf("\nWinners:\n")
		for potIdx, winnerSeats := range winners {
			if len(winnerSeats) > 0 {
				pot := h.Pots[potIdx]
				fmt.Printf("  Pot %d (%d chips): ", potIdx+1, pot.Amount)
				winnerNames := []string{}
				for _, seat := range winnerSeats {
					winnerNames = append(winnerNames, players[seat].Name)
				}
				fmt.Printf("%s\n", strings.Join(winnerNames, ", "))
			}
		}
	}

	fmt.Printf("\n=====================================\n")
	fmt.Printf("To reproduce: -seed %d -bots %d\n", *seed, *numBots)
	fmt.Printf("=====================================\n")
}

func selectAction(strategy string, validActions []game.Action, pot int, toCall int, chips int, rng *rand.Rand) (game.Action, int) {
	switch strategy {
	case "calling":
		// Always check or call
		for _, action := range validActions {
			if action == game.Check {
				return game.Check, 0
			}
		}
		for _, action := range validActions {
			if action == game.Call {
				return game.Call, 0
			}
		}
		return game.Fold, 0

	case "aggressive":
		// Raise 70% of the time if possible
		if rng.Float32() < 0.7 {
			for _, action := range validActions {
				if action == game.Raise {
					// Raise 2-3x pot
					amount := pot * (2 + rng.Intn(2))
					if amount < toCall*2 {
						amount = toCall * 2
					}
					if amount > chips {
						amount = chips
					}
					return game.Raise, amount
				}
			}
			for _, action := range validActions {
				if action == game.AllIn {
					return game.AllIn, 0
				}
			}
		}
		// Otherwise call/check
		for _, action := range validActions {
			if action == game.Call {
				return game.Call, 0
			}
			if action == game.Check {
				return game.Check, 0
			}
		}
		return game.Fold, 0

	default: // random
		if len(validActions) > 0 {
			action := validActions[rng.Intn(len(validActions))]
			if action == game.Raise {
				minRaise := toCall * 2
				maxRaise := pot * 2
				if maxRaise < minRaise {
					maxRaise = minRaise * 2
				}
				if maxRaise > chips {
					maxRaise = chips
				}
				if minRaise > chips {
					return game.Call, 0
				}
				amount := minRaise + rng.Intn(maxRaise-minRaise+1)
				return game.Raise, amount
			}
			return action, 0
		}
		return game.Fold, 0
	}
}

func formatAction(action game.Action, amount int, toCall int) string {
	switch action {
	case game.Fold:
		return "folds"
	case game.Check:
		return "checks"
	case game.Call:
		if toCall > 0 {
			return fmt.Sprintf("calls %d", toCall)
		}
		return "calls"
	case game.Raise:
		return fmt.Sprintf("raises to %d", amount)
	case game.AllIn:
		return "goes all-in"
	default:
		return action.String()
	}
}

func formatHand(hand game.Hand) string {
	cards := []string{}
	for i := uint(0); i < 52; i++ {
		card := game.Card(1 << i)
		if hand.HasCard(card) {
			cards = append(cards, card.String())
		}
	}
	return strings.Join(cards, " ")
}
