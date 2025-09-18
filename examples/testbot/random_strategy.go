package main

import "math/rand"

type RandomStrategy struct{}

func (s *RandomStrategy) GetName() string { return "random" }

func (s *RandomStrategy) SelectAction(validActions []string, pot int, toCall int, minBet int, chips int) (string, int) {
	if len(validActions) == 0 {
		return "fold", 0
	}
	action := validActions[rand.Intn(len(validActions))]
	if action == "raise" {
		amount := minBet
		if chips > minBet {
			amount = minBet + rand.Intn(chips-minBet+1)
		}
		return "raise", amount
	}
	return action, 0
}
