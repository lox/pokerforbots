package simple

import "github.com/lox/pokerforbots/sdk"

// CallBot always calls when possible, otherwise checks/folds
type CallBot struct {
	name string
}

// NewCallBot creates a new CallBot
func NewCallBot(name string) *CallBot {
	return &CallBot{name: name}
}

// MakeDecision implements the Agent interface - always calls when possible
func (cb *CallBot) MakeDecision(tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
	// Look for call action first
	for _, action := range validActions {
		if action.Action == sdk.ActionCall {
			return sdk.NewCallDecision("CallBot always calls")
		}
	}

	// If can't call, try to check
	for _, action := range validActions {
		if action.Action == sdk.ActionCheck {
			return sdk.NewCheckDecision("CallBot checks when can't call")
		}
	}

	// If can't call or check, fold
	return sdk.NewFoldDecision("CallBot folds when must bet")
}
