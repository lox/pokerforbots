package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"

	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
)

type CLI struct {
	Hands        []string `arg:"" help:"Player hands in format 'AcKd QhJs' (space separated, quoted)" required:"true"`
	Board        string   `short:"b" help:"Community board cards (e.g., 'Td7s8h')"`
	Possibilities bool    `short:"p" help:"Show detailed hand type probabilities"`
	Iterations   int      `short:"i" help:"Number of Monte Carlo iterations" default:"100000"`
	Seed         *int64   `help:"Random seed for reproducible results"`
}

var (
	// Style definitions
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	handStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("14"))

	winStyle = lipgloss.NewStyle().
		       Foreground(lipgloss.Color("10"))

	tieStyle = lipgloss.NewStyle().
		       Foreground(lipgloss.Color("11"))

	categoryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))

	percentStyle = lipgloss.NewStyle().
		       Foreground(lipgloss.Color("9"))
)

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)

	// Set up random seed
	var seed int64
	if cli.Seed != nil {
		seed = *cli.Seed
	} else {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	// Parse hands
	hands, err := parseHands(cli.Hands)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing hands: %v\n", err)
		ctx.Exit(1)
	}

	// Parse board
	var board []deck.Card
	if cli.Board != "" {
		board, err = deck.ParseCards(cli.Board)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing board: %v\n", err)
			ctx.Exit(1)
		}
		if len(board) > 5 {
			fmt.Fprintf(os.Stderr, "Board cannot have more than 5 cards\n")
			ctx.Exit(1)
		}
	}

	// Validate that no cards are duplicated
	if err := validateNoDuplicates(hands, board); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		ctx.Exit(1)
	}

	// Run calculations
	startTime := time.Now()
	results := calculateMonteCarlo(hands, board, cli.Iterations, rng)
	duration := time.Since(startTime)

	// Display results
	displayResults(results, board, cli.Possibilities, cli.Iterations, duration)
}

type PlayerResult struct {
	Hand         []deck.Card
	Wins         int
	Ties         int
	Total        int
	Possibilities map[string]int // For --possibilities mode
}

func parseHands(handStrings []string) ([][]deck.Card, error) {
	var hands [][]deck.Card
	
	for i, handStr := range handStrings {
		// Remove any extra spaces and parse
		handStr = strings.TrimSpace(handStr)
		hand, err := deck.ParseCards(strings.ReplaceAll(handStr, " ", ""))
		if err != nil {
			return nil, fmt.Errorf("hand %d: %v", i+1, err)
		}
		if len(hand) != 2 {
			return nil, fmt.Errorf("hand %d: must contain exactly 2 cards, got %d", i+1, len(hand))
		}
		hands = append(hands, hand)
	}
	
	return hands, nil
}

func validateNoDuplicates(hands [][]deck.Card, board []deck.Card) error {
	seen := make(map[deck.Card]bool)
	
	// Check board cards
	for _, card := range board {
		if seen[card] {
			return fmt.Errorf("duplicate card found: %s", card)
		}
		seen[card] = true
	}
	
	// Check hand cards
	for i, hand := range hands {
		for _, card := range hand {
			if seen[card] {
				return fmt.Errorf("duplicate card found in hand %d: %s", i+1, card)
			}
			seen[card] = true
		}
	}
	
	return nil
}

func calculateMonteCarlo(hands [][]deck.Card, board []deck.Card, iterations int, rng *rand.Rand) []PlayerResult {
	numPlayers := len(hands)
	results := make([]PlayerResult, numPlayers)
	
	// Initialize results
	for i := range results {
		results[i].Hand = hands[i]
		results[i].Total = iterations
		results[i].Possibilities = make(map[string]int)
	}
	
	// Create used cards set
	usedCards := evaluator.NewCardSet(board)
	for _, hand := range hands {
		for _, card := range hand {
			usedCards.Add(card)
		}
	}
	
	// Create available cards
	var availableCards []deck.Card
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			card := deck.Card{Suit: suit, Rank: rank}
			if !usedCards.Contains(card) {
				availableCards = append(availableCards, card)
			}
		}
	}
	
	// Run Monte Carlo simulation
	for iter := 0; iter < iterations; iter++ {
		// Complete the board if needed
		fullBoard := make([]deck.Card, len(board))
		copy(fullBoard, board)
		
		cardsNeeded := 5 - len(board)
		if cardsNeeded > 0 {
			// Randomly select cards for board completion
			selectedIndices := selectRandomIndices(len(availableCards), cardsNeeded, rng)
			for _, idx := range selectedIndices {
				fullBoard = append(fullBoard, availableCards[idx])
			}
		}
		
		// Evaluate all hands
		handRanks := make([]evaluator.HandRank, numPlayers)
		for i, hand := range hands {
			fullHand := make([]deck.Card, 7)
			copy(fullHand[:2], hand)
			copy(fullHand[2:], fullBoard)
			handRanks[i] = evaluator.Evaluate7(fullHand)
			
			// Track hand types for possibilities
			handType := handRanks[i].String()
			results[i].Possibilities[handType]++
		}
		
		// Determine winners
		bestRank := handRanks[0]
		for i := 1; i < numPlayers; i++ {
			if handRanks[i].Compare(bestRank) > 0 {
				bestRank = handRanks[i]
			}
		}
		
		// Count wins and ties
		winnersCount := 0
		for _, rank := range handRanks {
			if rank.Compare(bestRank) == 0 {
				winnersCount++
			}
		}
		
		for i, rank := range handRanks {
			if rank.Compare(bestRank) == 0 {
				if winnersCount == 1 {
					results[i].Wins++
				} else {
					results[i].Ties++
				}
			}
		}
	}
	
	return results
}



func selectRandomIndices(max, count int, rng *rand.Rand) []int {
	if count >= max {
		indices := make([]int, max)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	
	indices := make([]int, count)
	used := make(map[int]bool)
	
	for i := 0; i < count; i++ {
		for {
			idx := rng.Intn(max)
			if !used[idx] {
				indices[i] = idx
				used[idx] = true
				break
			}
		}
	}
	
	return indices
}

func displayResults(results []PlayerResult, board []deck.Card, showPossibilities bool, iterations int, duration time.Duration) {
	// Display header
	if len(board) > 0 {
		fmt.Printf("%s\n", headerStyle.Render("board"))
		fmt.Printf("%s\n\n", formatCards(board))
	}
	
	// Display hand results using tabwriter for proper alignment
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	
	// Header
	fmt.Fprintf(w, "%s\t%s\t%s\n", 
		headerStyle.Render("hand"),
		headerStyle.Render("win"),
		headerStyle.Render("tie"))
	
	// Results
	for _, result := range results {
		handStr := formatCards(result.Hand)
		winPct := float64(result.Wins) / float64(result.Total) * 100
		tiePct := float64(result.Ties) / float64(result.Total) * 100
		
		fmt.Fprintf(w, "%s\t%s\t%s\n", 
			handStyle.Render(handStr),
			winStyle.Render(fmt.Sprintf("%.1f%%", winPct)),
			tieStyle.Render(fmt.Sprintf("%.1f%%", tiePct)))
	}
	
	w.Flush()
	
	// Display possibilities breakdown if requested
	if showPossibilities && len(results) > 0 {
		fmt.Printf("\n")
		displayPossibilities(results)
	}
	
	// Display footer
	fmt.Printf("\n")
	fmt.Printf("%d iterations in %v\n", iterations, duration.Truncate(time.Millisecond))
}

func displayPossibilities(results []PlayerResult) {
	// Collect all possible hand types
	allTypes := make(map[string]bool)
	for _, result := range results {
		for handType := range result.Possibilities {
			allTypes[handType] = true
		}
	}
	
	// Order hand types by strength
	orderedTypes := []string{
		"Royal Flush", "Straight Flush", "Four of a Kind", "Full House",
		"Flush", "Straight", "Three of a Kind", "Two Pair", "One Pair", "High Card",
	}
	
	// Use tabwriter for proper alignment
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	
	// Display header
	fmt.Fprintf(w, "%s", categoryStyle.Render("hand"))
	for i := range results {
		fmt.Fprintf(w, "\t%s", handStyle.Render(formatCards(results[i].Hand)))
	}
	fmt.Fprintf(w, "\n")
	
	// Display each hand type
	for _, handType := range orderedTypes {
		if !allTypes[handType] {
			continue
		}
		
		fmt.Fprintf(w, "%s", categoryStyle.Render(handType))
		for _, result := range results {
			count := result.Possibilities[handType]
			pct := float64(count) / float64(result.Total) * 100
			if count > 0 {
				fmt.Fprintf(w, "\t%s", percentStyle.Render(fmt.Sprintf("%.1f%%", pct)))
			} else {
				fmt.Fprintf(w, "\t%s", percentStyle.Render("."))
			}
		}
		fmt.Fprintf(w, "\n")
	}
	
	w.Flush()
}

func formatCards(cards []deck.Card) string {
	var parts []string
	for _, card := range cards {
		parts = append(parts, card.String())
	}
	return strings.Join(parts, " ")
}
