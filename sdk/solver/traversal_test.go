package solver

import (
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/poker"
)

func mustParseHand(t *testing.T, cards ...string) poker.Hand {
	h, err := poker.ParseHand(cards...)
	if err != nil {
		t.Fatalf("parse hand %v: %v", cards, err)
	}
	return h
}

func TestUtilityForPlayerSidePot(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	hand := game.NewHandState(rng, []string{"Alice", "Bob", "Cara"}, 0, 5, 10, game.WithChips(1000))

	hand.Board = mustParseHand(t, "2h", "7d", "9c", "Jd", "Qs")
	hand.Street = game.Showdown

	players := hand.Players

	players[0].Bet = 100
	players[0].TotalBet = 100
	players[0].Chips = 0
	players[0].HoleCards = mustParseHand(t, "As", "Ad")

	players[1].Bet = 100
	players[1].TotalBet = 100
	players[1].Chips = 0
	players[1].HoleCards = mustParseHand(t, "Kc", "Kh")

	players[2].Bet = 40
	players[2].TotalBet = 40
	players[2].Chips = 0
	players[2].AllInFlag = true
	players[2].HoleCards = mustParseHand(t, "3c", "4c")

	hand.PotManager = game.NewPotManager(players)
	hand.PotManager.CollectBets(players)
	hand.PotManager.CalculateSidePots(players)

	pots := hand.GetPots()
	if len(pots) != 2 {
		t.Fatalf("expected 2 pots, got %d", len(pots))
	}
	if pots[0].Amount != 120 || len(pots[0].Eligible) != 3 {
		t.Fatalf("expected main pot 120 with 3 eligible, got %+v", pots[0])
	}
	if pots[1].Amount != 120 || len(pots[1].Eligible) != 2 {
		t.Fatalf("expected side pot 120 with 2 eligible, got %+v", pots[1])
	}

	if util := utilityForPlayer(hand, 0); util != 140 {
		t.Fatalf("expected P0 utility 140, got %d", util)
	}
	if util := utilityForPlayer(hand, 1); util != -100 {
		t.Fatalf("expected P1 utility -100, got %d", util)
	}
	if util := utilityForPlayer(hand, 2); util != -40 {
		t.Fatalf("expected P2 utility -40, got %d", util)
	}
}

func TestRaiseAmountsRespectsConstraints(t *testing.T) {
	abs := DefaultAbstraction()
	cfg := DefaultTrainingConfig()
	cfg.Players = 2
	cfg.SmallBlind = 1
	cfg.BigBlind = 2
	cfg.StartingStack = 10
	cfg.Iterations = 1

	trainer, err := NewTrainer(abs, cfg)
	if err != nil {
		t.Fatalf("new trainer: %v", err)
	}

	hand := game.NewHandState(rand.New(rand.NewSource(3)), []string{"A", "B"}, 0, cfg.SmallBlind, cfg.BigBlind, game.WithChips(cfg.StartingStack))

	hand.Betting.CurrentBet = cfg.BigBlind
	hand.Betting.MinRaise = cfg.BigBlind
	players := hand.Players

	for i := range players {
		players[i].Bet = cfg.BigBlind
		players[i].TotalBet = cfg.BigBlind
		players[i].Chips = cfg.StartingStack - cfg.BigBlind
	}

	hand.PotManager = game.NewPotManager(players)
	hand.PotManager.CollectBets(players)

	raises := trainer.raiseAmounts(hand, players[hand.ActivePlayer])
	want := []int{4, 5, 6}
	if len(raises) != len(want) {
		t.Fatalf("expected %d raises, got %d: %v", len(want), len(raises), raises)
	}
	for i, r := range raises {
		if r != want[i] {
			t.Fatalf("raises[%d]=%d, want %d", i, r, want[i])
		}
	}

	// Tight stack should block raises
	players[hand.ActivePlayer].Chips = 0
	if res := trainer.raiseAmounts(hand, players[hand.ActivePlayer]); len(res) != 0 {
		t.Fatalf("expected no raises with empty stack, got %v", res)
	}
}

func TestFilterRaisesPrunesToLimit(t *testing.T) {
	abs := DefaultAbstraction()
	abs.MaxRaisesPerBucket = 2
	cfg := DefaultTrainingConfig()
	cfg.Players = 2
	cfg.SmallBlind = 1
	cfg.BigBlind = 2
	cfg.StartingStack = 40
	cfg.Iterations = 1

	trainer, err := NewTrainer(abs, cfg)
	if err != nil {
		t.Fatalf("new trainer: %v", err)
	}

	hand := game.NewHandState(rand.New(rand.NewSource(17)), []string{"A", "B"}, 0, cfg.SmallBlind, cfg.BigBlind, game.WithChips(cfg.StartingStack))
	hand.ActivePlayer = 0
	hand.Betting.CurrentBet = cfg.BigBlind * 2
	hand.Betting.MinRaise = cfg.BigBlind
	players := hand.Players

	for i := range players {
		players[i].Bet = cfg.BigBlind * 2
		players[i].TotalBet = cfg.BigBlind * 2
		players[i].Chips = cfg.StartingStack - cfg.BigBlind*2
	}

	hand.PotManager = game.NewPotManager(players)
	hand.PotManager.CollectBets(players)

	amounts := trainer.raiseAmounts(hand, players[hand.ActivePlayer])
	if len(amounts) < 3 {
		t.Fatalf("expected multiple raise amounts, got %v", amounts)
	}

	filtered := trainer.filterRaises(hand, players[hand.ActivePlayer], amounts, false)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 raises after pruning, got %v", filtered)
	}
	if filtered[0] != amounts[0] {
		t.Fatalf("expected min raise %d to survive, got %d", amounts[0], filtered[0])
	}
	if filtered[1] != amounts[len(amounts)-1] {
		t.Fatalf("expected max raise %d to survive, got %d", amounts[len(amounts)-1], filtered[1])
	}

	abs.MaxRaisesPerBucket = 0
	trainerNoLimit, err := NewTrainer(abs, cfg)
	if err != nil {
		t.Fatalf("new trainer no limit: %v", err)
	}
	amounts2 := trainerNoLimit.raiseAmounts(hand, players[hand.ActivePlayer])
	filtered2 := trainerNoLimit.filterRaises(hand, players[hand.ActivePlayer], amounts2, false)
	if len(filtered2) != len(amounts2) {
		t.Fatalf("expected no pruning when limit disabled, got %v vs %v", filtered2, amounts2)
	}
}
