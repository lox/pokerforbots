package tui

import (
	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
)

// NetworkTUIAdapter adapts TUIModel to work with NetworkAgent
// This implements the TUIInterface required by NetworkAgent
type NetworkTUIAdapter struct {
	model *TUIModel
}

// NewNetworkTUIAdapter creates a new adapter
func NewNetworkTUIAdapter(model *TUIModel) *NetworkTUIAdapter {
	return &NetworkTUIAdapter{
		model: model,
	}
}

// AddLogEntry adds an entry to the game log
func (nta *NetworkTUIAdapter) AddLogEntry(entry string) {
	nta.model.AddLogEntry(entry)
}

// AddLogEntryAndScrollToShow adds an entry and scrolls to show it at the top
func (nta *NetworkTUIAdapter) AddLogEntryAndScrollToShow(entry string) {
	nta.model.AddLogEntryAndScrollToShow(entry)
}

// ClearLog clears the game log
func (nta *NetworkTUIAdapter) ClearLog() {
	nta.model.ClearLog()
}

// UpdatePot updates the current pot display value
func (nta *NetworkTUIAdapter) UpdatePot(pot int) {
	nta.model.UpdatePot(pot)
}

// UpdateCurrentBet updates the current bet display value
func (nta *NetworkTUIAdapter) UpdateCurrentBet(bet int) {
	nta.model.UpdateCurrentBet(bet)
}

// UpdateValidActions updates the valid actions display
func (nta *NetworkTUIAdapter) UpdateValidActions(actions []game.ValidAction) {
	nta.model.UpdateValidActions(actions)
}

// SetHumanTurn sets whether it's currently the human's turn to act
func (nta *NetworkTUIAdapter) SetHumanTurn(isHumansTurn bool, player *game.Player) {
	nta.model.SetHumanTurn(isHumansTurn, player)
}

// WaitForAction waits for user input (for use by NetworkAgent)
func (nta *NetworkTUIAdapter) WaitForAction() (string, []string, bool, error) {
	return nta.model.WaitForAction()
}

// FormatCards formats cards with colors - handles both []deck.Card and interface{}
func (nta *NetworkTUIAdapter) FormatCards(cards interface{}) string {
	switch c := cards.(type) {
	case []deck.Card:
		return nta.model.formatCards(c)
	default:
		// Try to convert from interface{} to []deck.Card
		if cardSlice, ok := cards.([]deck.Card); ok {
			return nta.model.formatCards(cardSlice)
		}
		return "" // Return empty string if conversion fails
	}
}
