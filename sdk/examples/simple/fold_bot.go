package simple

import "github.com/lox/pokerforbots/sdk"

// FoldBot always folds unless it can check
type FoldBot struct {
	name string
}

// NewFoldBot creates a new FoldBot
func NewFoldBot(name string) *FoldBot {
	return &FoldBot{name: name}
}

// MakeDecision implements the Agent interface - always folds unless can check
func (fb *FoldBot) MakeDecision(tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
	// Check if we can check (no bet to call)
	for _, action := range validActions {
		if action.Action == sdk.ActionCheck {
			return sdk.NewCheckDecision("FoldBot checks when free")
		}
	}

	// Otherwise fold
	return sdk.NewFoldDecision("FoldBot always folds")
}
