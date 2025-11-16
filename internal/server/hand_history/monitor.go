package handhistory

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/lox/pokerforbots/v2/internal/phh"
	"github.com/rs/zerolog"
)

const (
	defaultVariant  = "NT"
	defaultFilename = "session.phhs"
)

// Monitor records completed hands into PHH files with buffered writes.
type Monitor struct {
	cfg     MonitorConfig
	logger  zerolog.Logger
	clock   Clock
	outPath string

	mu                  sync.Mutex
	flushMu             sync.Mutex
	buffer              []*phh.HandHistory
	current             *handState
	seatContributions   []int
	flushNotifier       func()
	consecutiveFailures int
	disabled            bool
	sectionCounter      int
}

type handState struct {
	history         *phh.HandHistory
	seatToPlayerIdx []int
}

func (h *handState) playerIndex(seat int) int {
	if h == nil || seat < 0 || seat >= len(h.seatToPlayerIdx) {
		return -1
	}
	return h.seatToPlayerIdx[seat]
}

// NewMonitor constructs a monitor for a given game.
func NewMonitor(cfg MonitorConfig, logger zerolog.Logger) (*Monitor, error) {
	if cfg.GameID == "" {
		return nil, errors.New("handhistory: GameID is required")
	}
	if cfg.OutputDir == "" {
		return nil, errors.New("handhistory: OutputDir is required")
	}

	if cfg.Filename == "" {
		cfg.Filename = defaultFilename
	}
	if cfg.Variant == "" {
		cfg.Variant = defaultVariant
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("handhistory: create dir: %w", err)
	}

	outPath := filepath.Join(cfg.OutputDir, cfg.Filename)
	counter, err := readLastSectionCounter(outPath)
	if err != nil {
		return nil, fmt.Errorf("handhistory: read sections: %w", err)
	}

	m := &Monitor{
		cfg:            cfg,
		logger:         logger,
		clock:          cfg.Clock,
		outPath:        outPath,
		buffer:         make([]*phh.HandHistory, 0, max(1, cfg.FlushHands)),
		sectionCounter: counter,
	}
	return m, nil
}

// SetFlushNotifier registers a callback invoked when the monitor would like an async flush.
func (m *Monitor) SetFlushNotifier(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushNotifier = fn
}

// OnGameStart currently no-ops but kept for parity.
func (m *Monitor) OnGameStart(handLimit uint64) {}

// OnGameComplete flushes remaining hands synchronously.
func (m *Monitor) OnGameComplete(handsCompleted uint64, reason string) {
	_ = m.Flush()
}

// OnHandStart initializes tracking for a new hand.
func (m *Monitor) OnHandStart(handID string, players []Player, button int, blinds Blinds) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled {
		return
	}

	playerCount := len(players)
	if playerCount == 0 {
		return
	}

	m.ensureSeatContributions(playerCount)
	order := positionOrder(button, playerCount)
	seatToPlayerIdx := make([]int, playerCount)
	for i := range seatToPlayerIdx {
		seatToPlayerIdx[i] = -1
	}
	playerBySeat := make(map[int]Player, playerCount)
	for _, p := range players {
		playerBySeat[p.Seat] = p
	}

	history := &phh.HandHistory{
		Variant:           m.cfg.Variant,
		Table:             m.cfg.GameID,
		SeatCount:         playerCount,
		Seats:             make([]int, len(order)),
		Antes:             make([]int, len(order)),
		BlindsOrStraddles: make([]int, len(order)),
		MinBet:            blinds.Big,
		StartingStacks:    make([]int, len(order)),
		FinishingStacks:   make([]int, len(order)),
		Winnings:          make([]int, len(order)),
		Actions:           make([]string, 0, playerCount+16),
		Players:           make([]string, len(order)),
		HandID:            handID,
		Timestamp:         m.clock.Now(),
	}

	for pos, seatIdx := range order {
		player, ok := playerBySeat[seatIdx]
		if !ok && seatIdx >= 0 && seatIdx < len(players) {
			player = players[seatIdx]
			player.Seat = seatIdx
		}
		history.Seats[pos] = player.Seat + 1
		history.StartingStacks[pos] = player.Chips
		history.FinishingStacks[pos] = player.Chips
		history.Players[pos] = player.Name
		if player.Seat >= 0 && player.Seat < len(seatToPlayerIdx) {
			seatToPlayerIdx[player.Seat] = pos
		}
		if dealt := m.formatDealAction(pos, player.HoleCards); dealt != "" {
			history.Actions = append(history.Actions, dealt)
		}
	}

	assignBlinds(history.BlindsOrStraddles, seatToPlayerIdx, button, playerCount, blinds)
	m.current = &handState{history: history, seatToPlayerIdx: seatToPlayerIdx}
}

// OnPlayerAction records an action within the current street.
func (m *Monitor) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled || m.current == nil {
		return
	}
	if seat < 0 || seat >= len(m.seatContributions) {
		return
	}

	state := m.current
	phhIndex := state.playerIndex(seat)
	if phhIndex < 0 {
		return
	}
	m.seatContributions[seat] += amount
	total := m.seatContributions[seat]
	if formatted, ok := phh.FormatAction(phhIndex, action, total); ok && formatted != "" {
		state.history.Actions = append(state.history.Actions, formatted)
	}
}

// OnStreetChange updates board state and resets per-street contributions.
func (m *Monitor) OnStreetChange(handID string, street string, cards []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled || m.current == nil {
		return
	}

	m.ensureSeatContributions(len(m.seatContributions))
	normalized := phh.NormalizeCards(cards)
	if len(normalized) == 0 {
		return
	}
	prevLen := len(m.current.history.Board)
	if prevLen > len(normalized) {
		prevLen = len(normalized)
	}
	newCards := normalized[prevLen:]
	m.current.history.Board = append([]string(nil), normalized...)
	if len(newCards) > 0 {
		boardAction := fmt.Sprintf("d db %s", strings.Join(newCards, ""))
		m.current.history.Actions = append(m.current.history.Actions, boardAction)
	}
}

// OnHandComplete finalizes the hand and appends it to the buffer.
func (m *Monitor) OnHandComplete(outcome Outcome) {
	var notifier func()
	shouldNotify := false

	m.mu.Lock()
	if m.disabled || m.current == nil {
		m.mu.Unlock()
		return
	}

	state := m.current
	hist := state.history

	if outcome.Detail != nil {
		m.applyResultFields(state, outcome.Detail)
		m.appendShowdownActions(state, outcome.Detail.BotOutcomes)
	}

	populateTimeFields(hist)
	m.buffer = append(m.buffer, hist)
	m.current = nil

	if m.cfg.FlushHands > 0 && len(m.buffer) >= m.cfg.FlushHands && m.flushNotifier != nil {
		notifier = m.flushNotifier
		shouldNotify = true
	}
	m.mu.Unlock()

	if shouldNotify && notifier != nil {
		notifier()
	}
}

// Flush writes buffered hands to disk.
func (m *Monitor) Flush() error {
	m.flushMu.Lock()
	defer m.flushMu.Unlock()

	m.mu.Lock()
	if m.disabled || len(m.buffer) == 0 {
		m.mu.Unlock()
		return nil
	}
	hands := append([]*phh.HandHistory(nil), m.buffer...)
	baseSection := m.sectionCounter
	m.mu.Unlock()

	file, err := os.OpenFile(m.outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	written := 0
	lastSection := baseSection
	for i, hand := range hands {
		section := lastSection + 1
		if err := writeHand(file, section, hand, i < len(hands)-1); err != nil {
			m.finalizeFlush(written, lastSection)
			return err
		}
		lastSection = section
		written++
	}

	m.finalizeFlush(len(hands), lastSection)
	return nil
}

// Close flushes remaining data.
func (m *Monitor) Close() error {
	return m.Flush()
}

// HandleFlushResult updates state after a flush attempt and returns whether the monitor was disabled.
func (m *Monitor) HandleFlushResult(err error) (disabled bool, dropped int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		m.consecutiveFailures++
		if m.consecutiveFailures >= 3 {
			dropped = len(m.buffer)
			m.buffer = nil
			m.disabled = true
			return true, dropped
		}
		return false, 0
	}

	m.consecutiveFailures = 0
	return false, 0
}

// IsDisabled reports whether the monitor has been disabled.
func (m *Monitor) IsDisabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disabled
}

func (m *Monitor) ensureSeatContributions(n int) {
	if cap(m.seatContributions) < n {
		m.seatContributions = make([]int, n)
	} else {
		m.seatContributions = m.seatContributions[:n]
	}
	for i := range m.seatContributions {
		m.seatContributions[i] = 0
	}
}

func (m *Monitor) formatDealAction(seat int, holeCards []string) string {
	cards := "????"
	if m.cfg.IncludeHoleCards && len(holeCards) >= 2 {
		c1 := phh.NormalizeCard(holeCards[0])
		c2 := phh.NormalizeCard(holeCards[1])
		if c1 != "" && c2 != "" {
			cards = c1 + c2
		}
	}
	return fmt.Sprintf("d dh p%d %s", seat+1, cards)
}

func positionOrder(button, playerCount int) []int {
	order := make([]int, 0, playerCount)
	if playerCount <= 0 {
		return order
	}
	start := smallBlindSeat(button, playerCount)
	if start < 0 {
		start = 0
	}
	start = normalizeSeat(start, playerCount)
	for i := 0; i < playerCount; i++ {
		order = append(order, (start+i)%playerCount)
	}
	return order
}

func assignBlinds(target []int, seatToPlayerIdx []int, button, playerCount int, blinds Blinds) {
	if len(target) == 0 || playerCount == 0 {
		return
	}
	sbSeat := smallBlindSeat(button, playerCount)
	if sbSeat >= 0 && sbSeat < len(seatToPlayerIdx) {
		if idx := seatToPlayerIdx[sbSeat]; idx >= 0 && idx < len(target) {
			target[idx] = blinds.Small
		}
	}
	bbSeat := bigBlindSeat(button, playerCount)
	if bbSeat >= 0 && bbSeat < len(seatToPlayerIdx) {
		if idx := seatToPlayerIdx[bbSeat]; idx >= 0 && idx < len(target) {
			target[idx] = blinds.Big
		}
	}
}

func smallBlindSeat(button, playerCount int) int {
	if playerCount <= 0 {
		return -1
	}
	normalized := normalizeSeat(button, playerCount)
	if playerCount <= 2 {
		return normalized
	}
	return (normalized + 1) % playerCount
}

func bigBlindSeat(button, playerCount int) int {
	if playerCount <= 1 {
		return -1
	}
	normalized := normalizeSeat(button, playerCount)
	if playerCount == 2 {
		return (normalized + 1) % playerCount
	}
	return (normalized + 2) % playerCount
}

func normalizeSeat(seat, playerCount int) int {
	if playerCount <= 0 {
		return -1
	}
	seat %= playerCount
	if seat < 0 {
		seat += playerCount
	}
	return seat
}

func (m *Monitor) appendShowdownActions(state *handState, outcomes []BotOutcome) {
	if state == nil || len(outcomes) == 0 {
		return
	}
	hist := state.history
	for _, outcome := range outcomes {
		if !outcome.WentToShowdown || len(outcome.HoleCards) < 2 {
			continue
		}
		phhIdx := state.playerIndex(outcome.Seat)
		if phhIdx < 0 {
			continue
		}
		c1 := phh.NormalizeCard(outcome.HoleCards[0])
		c2 := phh.NormalizeCard(outcome.HoleCards[1])
		if c1 == "" || c2 == "" {
			continue
		}
		action := fmt.Sprintf("p%d sm %s%s", phhIdx+1, c1, c2)
		hist.Actions = append(hist.Actions, action)
	}
}

func (m *Monitor) applyResultFields(state *handState, detail *OutcomeDetail) {
	if state == nil || detail == nil {
		return
	}
	hist := state.history
	if len(hist.FinishingStacks) == 0 {
		hist.FinishingStacks = make([]int, len(hist.StartingStacks))
	}
	copy(hist.FinishingStacks, hist.StartingStacks)
	if len(hist.Winnings) == 0 {
		hist.Winnings = make([]int, len(hist.StartingStacks))
	} else if len(hist.Winnings) != len(hist.StartingStacks) {
		hist.Winnings = make([]int, len(hist.StartingStacks))
	}
	for _, outcome := range detail.BotOutcomes {
		phhIdx := state.playerIndex(outcome.Seat)
		if phhIdx < 0 || phhIdx >= len(hist.FinishingStacks) {
			continue
		}
		hist.FinishingStacks[phhIdx] += outcome.NetChips
		if outcome.NetChips > 0 {
			hist.Winnings[phhIdx] = outcome.NetChips
		}
	}
}

func populateTimeFields(hist *phh.HandHistory) {
	t := hist.Timestamp
	if t.IsZero() {
		return
	}
	utc := t.UTC()
	hist.Time = utc.Format("15:04:05")
	hist.TimeZone = "UTC"
	hist.Day = utc.Day()
	hist.Month = int(utc.Month())
	hist.Year = utc.Year()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *Monitor) finalizeFlush(flushed int, lastSection int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if flushed > 0 {
		if flushed >= len(m.buffer) {
			m.buffer = m.buffer[:0]
		} else {
			m.buffer = m.buffer[flushed:]
		}
	}
	if lastSection > m.sectionCounter {
		m.sectionCounter = lastSection
	}
}

func writeHand(file *os.File, section int, hand *phh.HandHistory, needBlank bool) error {
	if _, err := fmt.Fprintf(file, "[%d]\n", section); err != nil {
		return err
	}
	if err := phh.Encode(file, hand); err != nil {
		return err
	}
	if _, err := file.WriteString("\n"); err != nil {
		return err
	}
	if needBlank {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}
	return nil
}

func readLastSectionCounter(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	last := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) >= 3 && line[0] == '[' && line[len(line)-1] == ']' {
			if n, err := strconv.Atoi(line[1 : len(line)-1]); err == nil && n > last {
				last = n
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return last, nil
}
