package bot

import (
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/game"
)

// FoldBot is a simple bot that always folds (or checks when possible)
type FoldBot struct {
	logger *log.Logger
}

// NewFoldBot creates a new FoldBot instance
func NewFoldBot(logger *log.Logger) *FoldBot {
	return &FoldBot{logger: logger}
}

func (f *FoldBot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	// Always fold except when we can check
	for _, action := range validActions {
		if action.Action == game.Check {
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "fold-bot checking"}
		}
	}

	// Try to fold if it's a valid action
	for _, action := range validActions {
		if action.Action == game.Fold {
			return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "fold-bot folding"}
		}
	}

	// If we can't check or fold, just pick the first valid action
	if len(validActions) > 0 {
		return game.Decision{Action: validActions[0].Action, Amount: validActions[0].MinAmount, Reasoning: "fold-bot fallback"}
	}

	// This should never happen if validActions is properly populated
	return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "fold-bot emergency"}
}
