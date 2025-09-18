package main

type CallingStationStrategy struct{}

func (s *CallingStationStrategy) GetName() string { return "calling-station" }

func (s *CallingStationStrategy) SelectAction(validActions []string, pot int, toCall int, minBet int, chips int) (string, int) {
	for _, action := range validActions {
		if action == "check" {
			return "check", 0
		}
	}
	for _, action := range validActions {
		if action == "call" {
			return "call", 0
		}
	}
	return "fold", 0
}
