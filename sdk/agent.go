package sdk

// Agent represents a poker decision-making entity (bot)
type Agent interface {
	// MakeDecision takes the current table state and valid actions,
	// and returns a decision for what action to take
	MakeDecision(tableState TableState, validActions []ValidAction) Decision
}

// The Decision and ValidAction types are now defined in enums.go

// NewDecision creates a new decision with the specified action and amount
func NewDecision(action Action, amount int, reasoning ...string) Decision {
	decision := Decision{
		Action: action,
		Amount: amount,
	}

	if len(reasoning) > 0 {
		decision.Reasoning = reasoning[0]
	}

	return decision
}

// NewFoldDecision creates a fold decision
func NewFoldDecision(reasoning ...string) Decision {
	return NewDecision(ActionFold, 0, reasoning...)
}

// NewCallDecision creates a call decision
func NewCallDecision(reasoning ...string) Decision {
	return NewDecision(ActionCall, 0, reasoning...)
}

// NewCheckDecision creates a check decision
func NewCheckDecision(reasoning ...string) Decision {
	return NewDecision(ActionCheck, 0, reasoning...)
}

// NewRaiseDecision creates a raise decision with the specified amount
func NewRaiseDecision(amount int, reasoning ...string) Decision {
	return NewDecision(ActionRaise, amount, reasoning...)
}

// NewAllInDecision creates an all-in decision
func NewAllInDecision(reasoning ...string) Decision {
	return NewDecision(ActionAllIn, 0, reasoning...)
}
