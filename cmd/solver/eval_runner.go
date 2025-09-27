package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/sdk/spawner"
	"github.com/rs/zerolog"
)

type evaluationOptions struct {
	BlueprintPath string
	Hands         int
	Seed          int64
	SmallBlind    int
	BigBlind      int
	StartChips    int
	TimeoutMs     int
}

type evalResult struct {
	HandsCompleted uint64
	Duration       time.Duration
	Players        []evalPlayer
}

type evalPlayer struct {
	Name      string
	NetChips  int
	BBPerHand float64
	BBPer100  float64
	Hands     int
}

type gameStatsDTO struct {
	HandsCompleted uint64    `json:"hands_completed"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	SmallBlind     int       `json:"small_blind"`
	BigBlind       int       `json:"big_blind"`
	Players        []struct {
		DisplayName   string  `json:"display_name"`
		NetChips      int     `json:"net_chips"`
		AvgPerHand    float64 `json:"avg_per_hand"`
		DetailedStats *struct {
			BBPer100 float64 `json:"bb_per_100"`
			Hands    int     `json:"hands"`
		} `json:"detailed_stats"`
	} `json:"players"`
}

func runEvaluation(ctx context.Context, logger zerolog.Logger, opts evaluationOptions) (*evalResult, error) {
	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	srvCfg := server.Config{
		SmallBlind:  opts.SmallBlind,
		BigBlind:    opts.BigBlind,
		StartChips:  opts.StartChips,
		Timeout:     time.Duration(opts.TimeoutMs) * time.Millisecond,
		MinPlayers:  2,
		MaxPlayers:  2,
		Seed:        seed,
		HandLimit:   uint64(opts.Hands),
		EnableStats: true,
	}

	srv := server.NewServer(logger, rand.New(rand.NewSource(seed)), server.WithConfig(srvCfg))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	go srv.Serve(listener)

	baseURL := fmt.Sprintf("http://%s", listener.Addr())
	if err := server.WaitForHealthy(ctx, baseURL); err != nil {
		srv.Shutdown(ctx)
		return nil, fmt.Errorf("server start: %w", err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", listener.Addr())

	botSpawner := spawner.NewWithSeed(wsURL, logger, seed)
	defer botSpawner.StopAll()

	specs := []spawner.BotSpec{
		{
			Command: "go",
			Args:    []string{"run", "./sdk/examples/calling-station"},
			Count:   1,
		},
		{
			Command: "go",
			Args:    []string{"run", "./sdk/examples/complex"},
			Count:   1,
			Env: map[string]string{
				"POKERFORBOTS_BLUEPRINT":           opts.BlueprintPath,
				"POKERFORBOTS_BLUEPRINT_FAIL_HARD": "1",
			},
		},
	}

	if err := botSpawner.Spawn(specs...); err != nil {
		srv.Shutdown(ctx)
		return nil, fmt.Errorf("spawn bots: %w", err)
	}

	done := srv.DefaultGameDone()
	select {
	case <-ctx.Done():
		srv.Shutdown(ctx)
		return nil, ctx.Err()
	case <-done:
	}

	// Stop bots first since they don't exit automatically when the game ends
	if err := botSpawner.StopAll(); err != nil {
		logger.Warn().Err(err).Msg("bot stop warning")
	}

	metrics, haveMetrics := srv.DefaultGameMetrics()

	stats, err := fetchGameStats(baseURL)
	if err != nil {
		srv.Shutdown(ctx)
		return nil, err
	}

	if err := srv.Shutdown(ctx); err != nil {
		logger.Warn().Err(err).Msg("server shutdown warning")
	}

	duration := time.Duration(0)
	if haveMetrics {
		if !metrics.EndTime.IsZero() && !metrics.StartTime.IsZero() {
			duration = metrics.EndTime.Sub(metrics.StartTime)
		}
	} else if !stats.StartTime.IsZero() && !stats.EndTime.IsZero() {
		duration = stats.EndTime.Sub(stats.StartTime)
	}

	players := make([]evalPlayer, 0, len(stats.Players))
	for _, p := range stats.Players {
		bbPerHand := p.AvgPerHand / float64(opts.BigBlind)
		bbPer100 := 0.0
		hands := 0
		if p.DetailedStats != nil {
			bbPer100 = p.DetailedStats.BBPer100
			hands = p.DetailedStats.Hands
		}
		players = append(players, evalPlayer{
			Name:      p.DisplayName,
			NetChips:  p.NetChips,
			BBPerHand: bbPerHand,
			BBPer100:  bbPer100,
			Hands:     hands,
		})
	}

	return &evalResult{
		HandsCompleted: stats.HandsCompleted,
		Duration:       duration,
		Players:        players,
	}, nil
}

func fetchGameStats(baseURL string) (*gameStatsDTO, error) {
	url := fmt.Sprintf("%s/admin/games/default/stats", baseURL)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch stats: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch stats: unexpected status %d", resp.StatusCode)
	}
	var dto gameStatsDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	return &dto, nil
}
