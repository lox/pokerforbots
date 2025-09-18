package main

import "math/rand"

type AggressiveStrategy struct{}

func (s *AggressiveStrategy) GetName() string { return "aggressive" }

func (s *AggressiveStrategy) SelectAction(validActions []string, pot int, toCall int, minBet int, chips int) (string, int) {
	canRaise := false
	canAllIn := false
	for _, action := range validActions {
		if action == "raise" {
			canRaise = true
		}
		if action == "allin" {
			canAllIn = true
		}
	}

	if (canRaise || canAllIn) && rand.Float32() < 0.7 {
		if canAllIn {
			return "allin", 0
		}
		if canRaise {
			amount := pot*2 + rand.Intn(pot*2+1)
			if amount < minBet {
				amount = minBet
			}
			if amount > chips {
				amount = chips
			}
			return "raise", amount
		}
	}

	for _, action := range validActions {
		if action == "call" {
			return "call", 0
		}
	}
	for _, action := range validActions {
		if action == "check" {
			return "check", 0
		}
	}
	return "fold", 0
}
