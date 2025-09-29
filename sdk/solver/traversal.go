package solver

import (
	"github.com/lox/pokerforbots/internal/randutil"

	"math"
	rand "math/rand/v2"
	"sort"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/poker"
)

type solverAction struct {
	action game.Action
	amount int
}

type iterationContext struct {
	trainer      *Trainer
	deckSeed     int64
	button       int
	playerNames  []string
	stats        *TraversalStats
	sampler      *rand.Rand
	deckRNG      *rand.Rand // Reusable RNG for deck operations
	fastRNG      PCG32      // Embedded PCG32 to avoid allocations
	updateOpts   RegretUpdateOptions
	deckTemplate poker.Deck
}

func (t *Trainer) traverse(ctx *iterationContext, path []solverAction, target int, depth int, reachPlayer, reachOthers float64) (float64, error) {
	if ctx.stats != nil {
		ctx.stats.NodesVisited++
		if depth > ctx.stats.MaxDepth {
			ctx.stats.MaxDepth = depth
		}
	}

	hand, err := t.simulatePath(ctx, path)
	if err != nil {
		return 0, err
	}

	if hand.IsComplete() {
		if ctx.stats != nil {
			ctx.stats.TerminalNodes++
		}
		return float64(utilityForPlayer(hand, target)), nil
	}

	current := hand.ActivePlayer
	if current == -1 {
		advanceToNextDecision(hand)
		if hand.IsComplete() {
			return float64(utilityForPlayer(hand, target)), nil
		}
		current = hand.ActivePlayer
	}

	key := t.infoSetKey(hand, current)
	expandRaises := t.shouldExpandRaises(key)
	actions := t.legalActions(hand, expandRaises)
	if len(actions) == 0 {
		if ctx.stats != nil {
			ctx.stats.TerminalNodes++
		}
		return float64(utilityForPlayer(hand, target)), nil
	}

	entry := t.regrets.Get(key, len(actions))
	strategy := entry.Strategy()

	if current == target {
		util := make([]float64, len(actions))
		nodeUtil := 0.0
		for i, act := range actions {
			nextPath := appendPath(path, act)
			u, err := t.traverse(ctx, nextPath, target, depth+1, reachPlayer, reachOthers*strategy[i])
			if err != nil {
				return 0, err
			}
			util[i] = u
			nodeUtil += strategy[i] * u
		}

		regrets := make([]float64, len(actions))
		for i := range actions {
			regrets[i] = (util[i] - nodeUtil) * reachOthers
		}
		entry.Update(regrets, strategy, reachPlayer, ctx.updateOpts)
		t.recordVisit(key)
		return nodeUtil, nil
	}

	if t.trainCfg.Sampling == SamplingModeFullTraversal {
		nodeUtil := 0.0
		total := 0.0
		for i, act := range actions {
			prob := strategy[i]
			if prob <= 0 {
				continue
			}
			nextPath := appendPath(path, act)
			u, err := t.traverse(ctx, nextPath, target, depth+1, reachPlayer, reachOthers*prob)
			if err != nil {
				return 0, err
			}
			nodeUtil += prob * u
			total += prob
		}
		if total <= 0 && len(actions) > 0 {
			fallback := 1.0 / float64(len(actions))
			for _, act := range actions {
				nextPath := appendPath(path, act)
				u, err := t.traverse(ctx, nextPath, target, depth+1, reachPlayer, reachOthers*fallback)
				if err != nil {
					return 0, err
				}
				nodeUtil += fallback * u
			}
		}
		return nodeUtil, nil
	}

	sampled := strategy[:len(actions)]
	idx, prob := sampleStrategyIndex(sampled, ctx.sampler)
	if prob <= 0 {
		prob = 1.0 / float64(len(actions))
	}
	nextPath := appendPath(path, actions[idx])
	u, err := t.traverse(ctx, nextPath, target, depth+1, reachPlayer*prob, reachOthers)
	if err != nil {
		return 0, err
	}
	return u, nil
}

func (t *Trainer) simulatePath(ctx *iterationContext, path []solverAction) (*game.HandState, error) {
	deck := cloneDeck(&ctx.deckTemplate)
	hand := game.NewHandState(ctx.deckRNG, ctx.playerNames, ctx.button, t.trainCfg.SmallBlind, t.trainCfg.BigBlind, game.WithChips(t.trainCfg.StartingStack), game.WithDeck(deck))

	for _, step := range path {
		if hand.IsComplete() {
			break
		}
		if err := hand.ProcessAction(step.action, step.amount); err != nil {
			return nil, err
		}
	}

	advanceToNextDecision(hand)
	return hand, nil
}

func advanceToNextDecision(hand *game.HandState) {
	for !hand.IsComplete() && hand.ActivePlayer == -1 {
		hand.NextStreet()
	}
}

func appendPath(path []solverAction, act solverAction) []solverAction {
	next := make([]solverAction, len(path)+1)
	copy(next, path)
	next[len(path)] = act
	return next
}

func cloneDeck(src *poker.Deck) *poker.Deck {
	clone := *src
	return &clone
}

func (t *Trainer) legalActions(hand *game.HandState, expandRaises bool) []solverAction {
	raw := hand.GetValidActions()
	actions := make([]solverAction, 0, len(raw)+len(t.absCfg.BetSizing)+1)

	includeFold := false
	includeCheck := false
	includeCall := false
	includeAllIn := false
	includeRaise := false

	allowRaises := t.raisesEnabled()

	for _, act := range raw {
		switch act {
		case game.Fold:
			includeFold = true
		case game.Check:
			includeCheck = true
		case game.Call:
			includeCall = true
		case game.AllIn:
			if allowRaises {
				includeAllIn = true
			}
		case game.Raise:
			if allowRaises {
				includeRaise = true
			}
		}
	}

	if includeFold {
		actions = append(actions, solverAction{action: game.Fold})
	}
	if includeCheck {
		actions = append(actions, solverAction{action: game.Check})
	}
	if includeCall {
		actions = append(actions, solverAction{action: game.Call})
	}

	if allowRaises && includeRaise && hand.ActivePlayer >= 0 {
		player := hand.Players[hand.ActivePlayer]
		amounts := t.raiseAmounts(hand, player)
		amounts = t.filterRaises(hand, player, amounts, expandRaises)
		for _, total := range amounts {
			actions = append(actions, solverAction{action: game.Raise, amount: total})
		}
	}

	if allowRaises && includeAllIn {
		actions = append(actions, solverAction{action: game.AllIn})
	}

	if len(actions) > t.absCfg.MaxActionsPerNode {
		actions = actions[:t.absCfg.MaxActionsPerNode]
	}
	return actions
}

func (t *Trainer) filterRaises(hand *game.HandState, player *game.Player, totals []int, expand bool) []int {
	if expand {
		return totals
	}
	maxRaises := t.absCfg.MaxRaisesPerBucket
	if maxRaises <= 0 || len(totals) <= maxRaises {
		return totals
	}
	selected := make(map[int]struct{}, maxRaises)
	selectIndex := func(idx int) {
		if idx < 0 || idx >= len(totals) {
			return
		}
		if len(selected) >= maxRaises {
			return
		}
		if _, ok := selected[idx]; ok {
			return
		}
		selected[idx] = struct{}{}
	}
	selectIndex(0)
	if len(selected) < maxRaises {
		selectIndex(len(totals) - 1)
	}
	if len(selected) < maxRaises {
		selectIndex(t.closestRaiseIndex(hand, player, totals))
	}
	for i := 0; len(selected) < maxRaises && i < len(totals); i++ {
		selectIndex(i)
	}
	result := make([]int, 0, maxRaises)
	for i := 0; i < len(totals) && len(result) < maxRaises; i++ {
		if _, ok := selected[i]; ok {
			result = append(result, totals[i])
		}
	}
	return result
}

func (t *Trainer) closestRaiseIndex(hand *game.HandState, player *game.Player, totals []int) int {
	if len(totals) == 0 {
		return -1
	}
	toCall := 0
	if hand.Betting.CurrentBet > player.Bet {
		toCall = hand.Betting.CurrentBet - player.Bet
	}
	potTarget := hand.Betting.CurrentBet + toCall + t.potSize(hand) + toCall
	bestIdx := 0
	bestDiff := absInt(totals[0] - potTarget)
	for i := 1; i < len(totals); i++ {
		diff := absInt(totals[i] - potTarget)
		if diff < bestDiff {
			bestIdx = i
			bestDiff = diff
		}
	}
	return bestIdx
}

func (t *Trainer) raiseAmounts(hand *game.HandState, player *game.Player) []int {
	if !t.raisesEnabled() {
		return nil
	}
	maxTotal := player.Bet + player.Chips
	if maxTotal <= hand.Betting.CurrentBet {
		return nil
	}

	pot := t.potSize(hand)
	minRaise := hand.Betting.MinRaise
	if minRaise <= 0 {
		minRaise = t.trainCfg.BigBlind
		if minRaise <= 0 {
			minRaise = 1
		}
	}

	amounts := make([]int, 0, len(t.absCfg.BetSizing))
	seen := make(map[int]struct{}, len(t.absCfg.BetSizing))

	for _, fraction := range t.absCfg.BetSizing {
		if fraction <= 0 {
			continue
		}
		raise := int(math.Round(float64(pot) * fraction))
		if raise < minRaise {
			raise = minRaise
		}
		total := hand.Betting.CurrentBet + raise
		if total <= hand.Betting.CurrentBet {
			continue
		}
		if total >= maxTotal {
			continue
		}
		if _, ok := seen[total]; ok {
			continue
		}
		seen[total] = struct{}{}
		amounts = append(amounts, total)
	}

	sort.Ints(amounts)
	return amounts
}

func (t *Trainer) infoSetKey(hand *game.HandState, seat int) InfoSetKey {
	player := hand.Players[seat]

	holeBucket := t.bucket.HoleBucket(player.HoleCards)
	boardBucket := 0
	if hand.Board != 0 && hand.Board.CountCards() >= 3 {
		boardBucket = t.bucket.BoardBucket(hand.Board)
	}

	pot := t.potSize(hand)
	toCall := 0
	if hand.Betting.CurrentBet > player.Bet {
		toCall = hand.Betting.CurrentBet - player.Bet
	}

	return InfoSetKey{
		Street:       mapStreet(hand.Street),
		Player:       seat,
		HoleBucket:   holeBucket,
		BoardBucket:  boardBucket,
		PotBucket:    t.potBucket(pot),
		ToCallBucket: t.toCallBucket(toCall),
	}
}

func (t *Trainer) potSize(hand *game.HandState) int {
	pots := hand.GetPots()
	total := 0
	for _, pot := range pots {
		total += pot.Amount
	}
	return total
}

func (t *Trainer) potBucket(pot int) int {
	bb := max(t.trainCfg.BigBlind, 1)
	thresholds := []int{bb, bb * 3, bb * 6, bb * 12}
	for i, boundary := range thresholds {
		if pot <= boundary {
			return i
		}
	}
	return len(thresholds)
}

func (t *Trainer) toCallBucket(toCall int) int {
	bb := max(t.trainCfg.BigBlind, 1)
	thresholds := []int{0, bb, bb * 2, bb * 4}
	for i, boundary := range thresholds {
		if toCall <= boundary {
			return i
		}
	}
	return len(thresholds)
}

func mapStreet(s game.Street) Street {
	switch s {
	case game.Preflop:
		return StreetPreflop
	case game.Flop:
		return StreetFlop
	case game.Turn:
		return StreetTurn
	case game.River:
		return StreetRiver
	default:
		return StreetRiver
	}
}

func utilityForPlayer(hand *game.HandState, seat int) int {
	winnings := 0
	potList := hand.GetPots()
	winners := hand.GetWinners()

	for idx, pot := range potList {
		winnersForPot, ok := winners[idx]
		if !ok || len(winnersForPot) == 0 {
			continue
		}
		share := pot.Amount / len(winnersForPot)
		for _, w := range winnersForPot {
			if w == seat {
				winnings += share
			}
		}
	}

	contribution := hand.Players[seat].TotalBet
	return winnings - contribution
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sampleStrategyIndex(strategy []float64, rng *rand.Rand) (int, float64) {
	if len(strategy) == 0 {
		return 0, 0
	}
	if rng == nil {
		rng = randutil.New(42)
	}
	total := 0.0
	for _, v := range strategy {
		if v > 0 {
			total += v
		}
	}
	if total <= 0 {
		idx := rng.IntN(len(strategy))
		return idx, 1.0 / float64(len(strategy))
	}
	r := rng.Float64() * total
	acc := 0.0
	for i, v := range strategy {
		if v <= 0 {
			continue
		}
		acc += v
		if r <= acc {
			return i, v / total
		}
	}
	return len(strategy) - 1, strategy[len(strategy)-1] / total
}
