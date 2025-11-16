package server

import handhistory "github.com/lox/pokerforbots/v2/internal/server/hand_history"

// handHistoryAdapter converts server monitor callbacks into the handhistory package format.
type handHistoryAdapter struct {
	monitor *handhistory.Monitor
}

func newHandHistoryAdapter(m *handhistory.Monitor) HandMonitor {
	if m == nil {
		return nil
	}
	return &handHistoryAdapter{monitor: m}
}

func (h *handHistoryAdapter) OnGameStart(handLimit uint64) {
	h.monitor.OnGameStart(handLimit)
}

func (h *handHistoryAdapter) OnGameComplete(handsCompleted uint64, reason string) {
	h.monitor.OnGameComplete(handsCompleted, reason)
}

func (h *handHistoryAdapter) OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds) {
	h.monitor.OnHandStart(handID, convertPlayers(players), button, handhistory.Blinds{Small: blinds.Small, Big: blinds.Big})
}

func (h *handHistoryAdapter) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {
	h.monitor.OnPlayerAction(handID, seat, action, amount, stack)
}

func (h *handHistoryAdapter) OnStreetChange(handID string, street string, cards []string) {
	h.monitor.OnStreetChange(handID, street, append([]string(nil), cards...))
}

func (h *handHistoryAdapter) OnHandComplete(outcome HandOutcome) {
	h.monitor.OnHandComplete(convertOutcome(outcome))
}

func convertPlayers(players []HandPlayer) []handhistory.Player {
	if len(players) == 0 {
		return nil
	}
	converted := make([]handhistory.Player, len(players))
	for i, p := range players {
		converted[i] = handhistory.Player{
			Seat:        p.Seat,
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Chips:       p.Chips,
			HoleCards:   append([]string(nil), p.HoleCards...),
		}
	}
	return converted
}

func convertOutcome(outcome HandOutcome) handhistory.Outcome {
	var detail *handhistory.OutcomeDetail
	if outcome.Detail != nil {
		detail = &handhistory.OutcomeDetail{
			Board:       append([]string(nil), outcome.Detail.Board...),
			TotalPot:    outcome.Detail.TotalPot,
			BotOutcomes: convertBotOutcomes(outcome.Detail.BotOutcomes),
		}
	}

	return handhistory.Outcome{
		HandID:         outcome.HandID,
		HandsCompleted: outcome.HandsCompleted,
		HandLimit:      outcome.HandLimit,
		Detail:         detail,
	}
}

func convertBotOutcomes(bots []BotHandOutcome) []handhistory.BotOutcome {
	if len(bots) == 0 {
		return nil
	}
	converted := make([]handhistory.BotOutcome, len(bots))
	for i, b := range bots {
		name := ""
		if b.Bot != nil {
			name = b.Bot.ID
		}
		converted[i] = handhistory.BotOutcome{
			Name:           name,
			Seat:           b.Position,
			NetChips:       b.NetChips,
			HoleCards:      append([]string(nil), b.HoleCards...),
			Won:            b.WonAtShowdown || b.NetChips > 0,
			WentToShowdown: b.WentToShowdown,
		}
	}
	return converted
}
