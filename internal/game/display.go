package game

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
)

// DisplayStyles contains styling for game display
type DisplayStyles struct {
	Header       lipgloss.Style
	SubHeader    lipgloss.Style
	Action       lipgloss.Style
	Winner       lipgloss.Style
	CardRed      lipgloss.Style
	CardBlack    lipgloss.Style
	Pot          lipgloss.Style
	Separator    lipgloss.Style
	PlayerHeader lipgloss.Style // for "(You)"
	PlayerInfo   lipgloss.Style // for "(AI)"
	Street       lipgloss.Style // for "*** PRE-FLOP ***"
}

// NewDisplayStyles creates a new set of display styles
func NewDisplayStyles() *DisplayStyles {
	return &DisplayStyles{
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			Bold(true),
		SubHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true),
		Action: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#74B9FF")),
		Winner: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true),
		CardRed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true),
		CardBlack: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Bold(true),
		Pot: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true),
		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")),
		PlayerHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true),
		PlayerInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")),
		Street: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true),
	}
}

// HandDisplay manages the display of poker hands
type HandDisplay struct {
	styles *DisplayStyles
}

// NewHandDisplay creates a new hand display manager
func NewHandDisplay() *HandDisplay {
	return &HandDisplay{
		styles: NewDisplayStyles(),
	}
}

func (hd *HandDisplay) ShowHandHeader(seats int, table *Table) {
	// Show compact hand header
	fmt.Printf("Hand #%d • %d players • $%d/$%d\n",
		table.HandNumber, seats, table.SmallBlind, table.BigBlind)

}

// ShowBettingRoundTransition displays the transition to a new betting round
func (hd *HandDisplay) ShowBettingRoundTransition(table *Table) {
	fmt.Println()

	switch table.CurrentRound {
	case Flop:
		fmt.Println(hd.styles.Header.Render("*** FLOP ***"))
		if len(table.CommunityCards) >= 3 {
			flop := table.CommunityCards[:3]
			fmt.Printf("Board: %s\n", hd.formatCards(flop))
		}
	case Turn:
		fmt.Println(hd.styles.Header.Render("*** TURN ***"))
		if len(table.CommunityCards) >= 4 {
			turn := table.CommunityCards[3]
			var turnStr string
			if turn.IsRed() {
				turnStr = hd.styles.CardRed.Render(turn.String())
			} else {
				turnStr = hd.styles.CardBlack.Render(turn.String())
			}
			fmt.Printf("Board: %s [%s]\n",
				hd.formatCards(table.CommunityCards[:3]),
				turnStr)
		}
	case River:
		fmt.Println(hd.styles.Header.Render("*** RIVER ***"))
		if len(table.CommunityCards) >= 5 {
			river := table.CommunityCards[4]
			var riverStr string
			if river.IsRed() {
				riverStr = hd.styles.CardRed.Render(river.String())
			} else {
				riverStr = hd.styles.CardBlack.Render(river.String())
			}
			fmt.Printf("Board: %s [%s]\n",
				hd.formatCards(table.CommunityCards[:4]),
				riverStr)
		}
	case Showdown:
		fmt.Println(hd.styles.Header.Render("*** SHOWDOWN ***"))
		fmt.Printf("Final Board: %s\n", hd.formatCards(table.CommunityCards))
	}

	fmt.Printf("Pot: %s\n", hd.styles.Pot.Render(fmt.Sprintf("$%d", table.Pot)))
	fmt.Println()
}

func (hd *HandDisplay) ShowPlayerPositions(table *Table) {
	for _, player := range table.Players {
		playerType := ""
		if player.Type == Human {
			playerType = hd.styles.PlayerHeader.Render("(You)")
		} else {
			playerType = hd.styles.PlayerInfo.Render("(AI)")
		}

		// Add position indicator
		positionIndicator := ""
		if player.Position == Button {
			positionIndicator = hd.styles.PlayerHeader.Render("🔘 BTN")
		} else if player.Position == SmallBlind {
			positionIndicator = hd.styles.PlayerInfo.Render("SB")
		} else if player.Position == BigBlind {
			positionIndicator = hd.styles.PlayerInfo.Render("BB")
		} else if player.Position == UnderTheGun {
			positionIndicator = hd.styles.PlayerInfo.Render("UTG")
		} else if player.Position == Cutoff {
			positionIndicator = hd.styles.PlayerInfo.Render("CO")
		}

		if positionIndicator != "" {
			fmt.Printf("Seat %d: %s %s %s - $%d\n", player.SeatNumber, player.Name, playerType, positionIndicator, player.Chips)
		} else {
			fmt.Printf("Seat %d: %s %s - $%d\n", player.SeatNumber, player.Name, playerType, player.Chips)
		}
	}
	fmt.Println()
}

// ShowHoleCards shows the hole cards dealt phase
func (hd *HandDisplay) ShowHoleCards(table *Table) {
	fmt.Println(hd.styles.Street.Render("*** HOLE CARDS ***"))

	// Show hole cards dealt to human player
	for _, player := range table.ActivePlayers {
		if player.Type == Human {
			fmt.Printf("Dealt to %s: %s\n", player.Name, hd.formatHoleCards(player.HoleCards))
			break
		}
	}
}

// ShowBlindPosting shows the blind posting messages
func (hd *HandDisplay) ShowBlindPosting(table *Table) {
	for _, player := range table.ActivePlayers {
		if player.Position == SmallBlind && player.BetThisRound > 0 {
			fmt.Printf("%s: posts small blind $%d\n", player.Name, player.BetThisRound)
		} else if player.Position == BigBlind && player.BetThisRound > 0 {
			fmt.Printf("%s: posts big blind $%d\n", player.Name, player.BetThisRound)
		}
	}
}

// ShowStreet displays a betting street header
func (hd *HandDisplay) ShowStreet(street string) {
	fmt.Println()
	fmt.Println(hd.styles.Street.Render(fmt.Sprintf("*** %s ***", street)))
}

// formatHoleCards formats hole cards for display
func (hd *HandDisplay) formatHoleCards(cards []deck.Card) string {
	if len(cards) != 2 {
		return "[?? ??]"
	}

	result := "["
	for i, card := range cards {
		if i > 0 {
			result += " "
		}
		if card.IsRed() {
			result += hd.styles.CardRed.Render(card.String())
		} else {
			result += hd.styles.CardBlack.Render(card.String())
		}
	}
	result += "]"
	return result
}

// ShowTestGameSetup displays the test game setup information
func (hd *HandDisplay) ShowTestGameSetup(seats int) {
	fmt.Printf("🎮 Setting up %d-seat poker table\n", seats)
}

// ShowTestPlayerSeating displays player seating for test mode
func (hd *HandDisplay) ShowTestPlayerSeating(table *Table) {
	fmt.Printf("👥 Players seated:\n")
	for _, player := range table.Players {
		playerType := ""
		if player.Type == Human {
			playerType = hd.styles.PlayerHeader.Render("(You)")
		} else {
			playerType = hd.styles.PlayerInfo.Render("(AI)")
		}
		fmt.Printf("   Seat %d: %s %s - $%d\n", player.SeatNumber, player.Name, playerType, player.Chips)
	}
	fmt.Println()
}

// ShowTestHandStart displays hand start information for test mode
func (hd *HandDisplay) ShowTestHandStart(table *Table) {
	fmt.Println("🎯 Starting new hand...")
	fmt.Printf("Hand #%d - %s\n", table.HandNumber, table.CurrentRound)
	fmt.Printf("Pot: $%d\n", table.Pot)
}

// ShowTestPlayerPositions displays player positions for test mode
func (hd *HandDisplay) ShowTestPlayerPositions(table *Table) {
	fmt.Println("\n🪑 Player positions:")
	for _, player := range table.ActivePlayers {
		if player.Type == Human {
			fmt.Printf("   %s: %s %s\n", player.Name, player.Position, hd.formatHoleCards(player.HoleCards))
		} else {
			fmt.Printf("   %s: %s [🂠 🂠]\n", player.Name, player.Position)
		}
	}
}

// ShowTestBettingRounds displays betting round simulation for test mode
func (hd *HandDisplay) ShowTestBettingRounds() {
	fmt.Println("\n🎰 Simulating betting rounds...")
}

// ShowTestCommunityCards displays community cards for test mode
func (hd *HandDisplay) ShowTestCommunityCards(street string, cards []deck.Card) {
	fmt.Printf("%s: %s\n", street, hd.formatCards(cards))
}

// ShowTestFinalHands displays final hands for test mode
func (hd *HandDisplay) ShowTestFinalHands(table *Table) {
	fmt.Println("\n🏆 Final hands:")
	for _, player := range table.ActivePlayers {
		bestHand := player.GetBestHand(table.CommunityCards)
		fmt.Printf("   %s: %s\n", player.Name, bestHand.String())
	}
	fmt.Println()
}

// showHoleCardsProminent displays hole cards in a prominent way
func (hd *HandDisplay) showHoleCardsProminent(player *Player) {
	if len(player.HoleCards) != 2 {
		return
	}

	// Create a bordered box for hole cards
	cardBox := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("#FFD700")).
		Padding(1, 2).
		Align(lipgloss.Center)

	card1 := player.HoleCards[0]
	card2 := player.HoleCards[1]

	var card1Str, card2Str string
	if card1.IsRed() {
		card1Str = hd.styles.CardRed.Render(card1.String())
	} else {
		card1Str = hd.styles.CardBlack.Render(card1.String())
	}

	if card2.IsRed() {
		card2Str = hd.styles.CardRed.Render(card2.String())
	} else {
		card2Str = hd.styles.CardBlack.Render(card2.String())
	}

	holeCardsText := fmt.Sprintf("YOUR HOLE CARDS\n\n    %s    %s", card1Str, card2Str)
	fmt.Println(cardBox.Render(holeCardsText))
}

// findHumanPlayer finds the human player at the table
func (hd *HandDisplay) findHumanPlayer(table *Table) *Player {
	for _, player := range table.ActivePlayers {
		if player.Type == Human {
			return player
		}
	}
	return nil
}

// ShowPlayerAction displays a player's action
func (hd *HandDisplay) ShowPlayerAction(player *Player, table *Table) {
	var actionText string

	switch player.LastAction {
	case Call:
		actionText = fmt.Sprintf("%s: calls $%d", player.Name, player.ActionAmount)
	case Raise:
		actionText = fmt.Sprintf("%s: raises $%d to $%d", player.Name,
			player.ActionAmount, player.TotalBet)
	case AllIn:
		actionText = fmt.Sprintf("%s: bets $%d and is all-in", player.Name, player.ActionAmount)
	case Fold:
		actionText = fmt.Sprintf("%s: folds", player.Name)
	case Check:
		actionText = fmt.Sprintf("%s: checks", player.Name)
	default:
		actionText = fmt.Sprintf("%s %s", player.Name, player.LastAction)
		if player.ActionAmount > 0 {
			actionText += fmt.Sprintf(" $%d", player.ActionAmount)
		}
	}

	// Add pot info for significant actions
	if player.LastAction == Raise || player.LastAction == AllIn {
		actionText += fmt.Sprintf(" (pot now: $%d)", table.Pot)
	}

	fmt.Println(hd.styles.Action.Render(actionText))
}

// ShowCompleteShowdown displays all players' hands and determines winner
func (hd *HandDisplay) ShowCompleteShowdown(table *Table) {
	fmt.Println()
	fmt.Println(hd.styles.Header.Render("*** SHOWDOWN ***"))
	fmt.Printf("Final Board: %s\n", hd.formatCards(table.CommunityCards))
	fmt.Println()

	// Collect all players still in the hand
	var playersInHand []*Player
	for _, player := range table.ActivePlayers {
		if player.IsInHand() {
			playersInHand = append(playersInHand, player)
		}
	}

	if len(playersInHand) <= 1 {
		// Only one player left - they win by default
		if len(playersInHand) == 1 {
			winner := playersInHand[0]
			fmt.Printf("%s wins $%d (all others folded)\n",
				hd.styles.Winner.Render(winner.Name), table.Pot)
		}
		return
	}

	// Evaluate all hands and show them
	type playerHand struct {
		player *Player
		hand   evaluator.Hand
	}

	var hands []playerHand
	for _, player := range playersInHand {
		hand := player.GetBestHand(table.CommunityCards)
		hands = append(hands, playerHand{player, hand})

		// Show each player's cards and hand with detailed description
		holeCards := hd.formatCards(player.HoleCards)
		handDescription := hd.getDetailedHandDescription(hand)
		fmt.Printf("%s shows %s (%s)\n",
			player.Name, holeCards, handDescription)
	}

	// Find the winner(s)
	bestHand := hands[0].hand
	var winners []*Player
	winners = append(winners, hands[0].player)

	for i := 1; i < len(hands); i++ {
		comparison := hands[i].hand.Compare(bestHand)
		if comparison > 0 {
			// New best hand
			bestHand = hands[i].hand
			winners = []*Player{hands[i].player}
		} else if comparison == 0 {
			// Tie
			winners = append(winners, hands[i].player)
		}
	}

	fmt.Println()

	// Announce winner(s)
	if len(winners) == 1 {
		winner := winners[0]
		winAmount := table.Pot
		fmt.Printf("%s wins $%d with %s\n",
			hd.styles.Winner.Render(winner.Name),
			winAmount,
			bestHand.String())

		// Award chips
		winner.Chips += winAmount
	} else {
		// Split pot
		splitAmount := table.Pot / len(winners)
		remainder := table.Pot % len(winners)

		var winnerNames []string
		for i, winner := range winners {
			winnerNames = append(winnerNames, winner.Name)
			award := splitAmount
			if i < remainder {
				award++ // Give remainder to first winners
			}
			winner.Chips += award
		}

		fmt.Printf("Split pot: %s tie with %s ($%d each)\n",
			strings.Join(winnerNames, ", "),
			bestHand.String(),
			splitAmount)
	}
}

// ShowHandSummary displays a summary of the completed hand
func (hd *HandDisplay) ShowHandSummary(table *Table) {
	fmt.Println()
	fmt.Println(hd.styles.Separator.Render("═══════════════════════════════════════════════"))
	fmt.Printf("Hand #%d Complete\n", table.HandNumber)
	fmt.Printf("Final Board: %s\n", hd.formatCards(table.CommunityCards))

	// Show updated chip counts
	fmt.Println("\nChip Counts:")
	for _, player := range table.Players {
		fmt.Printf("  %s: $%d\n", player.Name, player.Chips)
	}

	fmt.Println(hd.styles.Separator.Render("═══════════════════════════════════════════════"))
	fmt.Println()
}

// ShowBettingRoundComplete displays when a betting round is complete
func (hd *HandDisplay) ShowBettingRoundComplete(table *Table) {
	activePlayers := 0
	for _, player := range table.ActivePlayers {
		if player.IsInHand() {
			activePlayers++
		}
	}

	if activePlayers <= 1 {
		fmt.Println(hd.styles.SubHeader.Render("--- All players folded ---"))
		return
	}

	roundName := table.CurrentRound.String()
	fmt.Println(hd.styles.SubHeader.Render(fmt.Sprintf("--- %s betting complete ---", roundName)))
}

// formatCards formats cards with proper colors
func (hd *HandDisplay) formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		if card.IsRed() {
			formatted = append(formatted, hd.styles.CardRed.Render(card.String()))
		} else {
			formatted = append(formatted, hd.styles.CardBlack.Render(card.String()))
		}
	}

	return "[" + strings.Join(formatted, " ") + "]"
}

// getDetailedHandDescription returns a detailed description of the hand
func (hd *HandDisplay) getDetailedHandDescription(hand evaluator.Hand) string {
	switch hand.Rank {
	case evaluator.HighCard:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("High Card (%s high)", hand.Kickers[0])
		}
		return "High Card"
	case evaluator.OnePair:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("Pair of %ss", hd.getRankName(hand.Kickers[0]))
		}
		return "One Pair"
	case evaluator.TwoPair:
		if len(hand.Kickers) >= 2 {
			return fmt.Sprintf("Two Pair (%ss and %ss)",
				hd.getRankName(hand.Kickers[0]),
				hd.getRankName(hand.Kickers[1]))
		}
		return "Two Pair"
	case evaluator.ThreeOfAKind:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("Three of a Kind (%ss)", hd.getRankName(hand.Kickers[0]))
		}
		return "Three of a Kind"
	case evaluator.Straight:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("Straight (%s high)", hand.Kickers[0])
		}
		return "Straight"
	case evaluator.Flush:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("Flush (%s high)", hand.Kickers[0])
		}
		return "Flush"
	case evaluator.FullHouse:
		if len(hand.Kickers) >= 2 {
			return fmt.Sprintf("Full House (%ss full of %ss)",
				hd.getRankName(hand.Kickers[0]),
				hd.getRankName(hand.Kickers[1]))
		}
		return "Full House"
	case evaluator.FourOfAKind:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("Four of a Kind (%ss)", hd.getRankName(hand.Kickers[0]))
		}
		return "Four of a Kind"
	case evaluator.StraightFlush:
		if len(hand.Kickers) > 0 {
			return fmt.Sprintf("Straight Flush (%s high)", hand.Kickers[0])
		}
		return "Straight Flush"
	case evaluator.RoyalFlush:
		return "Royal Flush"
	default:
		return hand.Rank.String()
	}
}

// getRankName returns the plural name for a rank
func (hd *HandDisplay) getRankName(rank deck.Rank) string {
	switch rank {
	case deck.Two:
		return "Two"
	case deck.Three:
		return "Three"
	case deck.Four:
		return "Four"
	case deck.Five:
		return "Five"
	case deck.Six:
		return "Six"
	case deck.Seven:
		return "Seven"
	case deck.Eight:
		return "Eight"
	case deck.Nine:
		return "Nine"
	case deck.Ten:
		return "Ten"
	case deck.Jack:
		return "Jack"
	case deck.Queen:
		return "Queen"
	case deck.King:
		return "King"
	case deck.Ace:
		return "Ace"
	default:
		return "Unknown"
	}
}
