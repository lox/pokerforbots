package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/rs/zerolog"
)

type CLI struct {
	Addr             string   `kong:"default=':8080',help='Server address'"`
	Debug            bool     `kong:"help='Enable debug logging'"`
	SmallBlind       int      `kong:"default='5',help='Small blind amount'"`
	BigBlind         int      `kong:"default='10',help='Big blind amount'"`
	StartChips       int      `kong:"default='1000',help='Starting chip count'"`
	TimeoutMs        int      `kong:"default='100',help='Decision timeout in milliseconds'"`
	MinPlayers       int      `kong:"default='2',help='Minimum players per hand'"`
	MaxPlayers       int      `kong:"default='9',help='Maximum players per hand'"`
	RequirePlayer    bool     `kong:"default='true',help='Require at least one player-role bot per hand'"`
	InfiniteBankroll bool     `kong:"default='false',help='Bots never run out of chips (for simulations)'"`
	NPCBots          int      `kong:"default='0',help='Total NPC bots to spawn in default game (auto distribution)'"`
	NPCCalling       int      `kong:"default='0',help='NPC calling-station bots (overrides auto distribution)'"`
	NPCRandom        int      `kong:"default='0',help='NPC random bots (overrides auto distribution)'"`
	NPCAggro         int      `kong:"default='0',help='NPC aggressive bots (overrides auto distribution)'"`
	Seed             *int64   `kong:"help='Deterministic RNG seed for the server (optional)'"`
	Hands            uint64   `kong:"default='0',help='Maximum hands to run in the default game (0 = unlimited)'"`
	CollectDetailed  bool     `kong:"name='collect-detailed-stats',default='false',help='Collect detailed statistics (impacts performance)'"`
	MaxStatsHands    int      `kong:"default='10000',help='Maximum hands to track in statistics (memory limit)'"`
	BotCmd           []string `kong:"help='Command to run a local bot; may be specified multiple times. Env: POKERFORBOTS_SERVER, POKERFORBOTS_GAME'"`
	NPCBotCmd        []string `kong:"name='npc-bot-cmd',help='Command to run an external NPC bot; may be specified multiple times. Env: POKERFORBOTS_SERVER, POKERFORBOTS_GAME, POKERFORBOTS_ROLE=npc'"`
	PrintStatsOnExit bool     `kong:"help='Print /admin/games/default/stats JSON on exit'"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("pokerforbots-server"),
		kong.Description("High-performance poker server for bot-vs-bot play"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	// Configure zerolog for pretty console output
	level := zerolog.InfoLevel
	if cli.Debug {
		level = zerolog.DebugLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().
		Timestamp().
		Logger()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create server configuration
	config := server.Config{
		SmallBlind:       cli.SmallBlind,
		BigBlind:         cli.BigBlind,
		StartChips:       cli.StartChips,
		Timeout:          time.Duration(cli.TimeoutMs) * time.Millisecond,
		MinPlayers:       cli.MinPlayers,
		MaxPlayers:       cli.MaxPlayers,
		RequirePlayer:    cli.RequirePlayer,
		InfiniteBankroll: cli.InfiniteBankroll,
		HandLimit:        cli.Hands,
		Seed:             0,
		EnableStats:      cli.CollectDetailed,
		MaxStatsHands:    cli.MaxStatsHands,
	}

	// Create RNG instance for server
	seed := time.Now().UnixNano()
	if cli.Seed != nil {
		seed = *cli.Seed
	}

	rng := rand.New(rand.NewSource(seed))
	config.Seed = seed
	srv := server.NewServerWithConfig(logger, rng, config)

	if specs := computeDefaultNPCSpecs(cli.NPCBots, cli.NPCCalling, cli.NPCRandom, cli.NPCAggro); len(specs) > 0 {
		srv.AddBootstrapNPCs("default", specs)
		for _, spec := range specs {
			logger.Info().Str("game_id", "default").Str("strategy", spec.Strategy).Int("count", spec.Count).Msg("Spawning default NPC bots")
		}
	}

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info().
			Str("addr", cli.Addr).
			Int("small_blind", cli.SmallBlind).
			Int("big_blind", cli.BigBlind).
			Int("start_chips", cli.StartChips).
			Int("timeout_ms", cli.TimeoutMs).
			Int("min_players", cli.MinPlayers).
			Int("max_players", cli.MaxPlayers).
			Bool("infinite_bankroll", cli.InfiniteBankroll).
			Uint64("hand_limit", cli.Hands).
			Int64("seed", seed).
			Bool("collect_detailed_stats", cli.CollectDetailed).
			Int("max_stats_hands", cli.MaxStatsHands).
			Msg("Server starting")
		serverErr <- srv.Start(cli.Addr)
	}()

	// Bootstrap external bots if requested (run each command)
	botProcsCtx, botProcsCancel := context.WithCancel(context.Background())
	defer botProcsCancel()
	var anyBotDone <-chan error
	var allBotsDone <-chan error
	{
		serverWS := toWSURL(cli.Addr)
		var playerChans []<-chan error
		var npcChans []<-chan error
		for _, cmd := range cli.BotCmd {
			playerChans = append(playerChans, spawnBot(logger, botProcsCtx, cmd, serverWS, "default"))
		}
		for _, cmd := range cli.NPCBotCmd {
			npcChans = append(npcChans, spawnBotWithRole(logger, botProcsCtx, cmd, serverWS, "default", "npc"))
		}
		// Only treat player bot exits as a shutdown trigger
		anyBotDone = firstDone(playerChans)
		// Wait for all bots (players and NPCs) on shutdown paths
		allBots := append([]<-chan error{}, playerChans...)
		allBots = append(allBots, npcChans...)
		allBotsDone = waitAll(allBots)
	}

	// Monitor for game completion if hand limit is set on default game
	var gameCompleteNotifier <-chan struct{}
	if cli.Hands > 0 {
		// Server will automatically shut down when default game reaches hand limit
		// This is useful for profiling and benchmarking scenarios
		gameCompleteNotifier = srv.DefaultGameDone()
		if gameCompleteNotifier != nil {
			logger.Info().Uint64("hand_limit", cli.Hands).Msg("Will exit when default game completes")
		}
	}

	// Wait for server error, interrupt signal, or game completion
	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			ctx.FatalIfErrorf(err)
		}
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully...")

		// If bots were started, wait for them to exit
		if allBotsDone != nil {
			logger.Info().Msg("Waiting for bot processes to exit...")
			<-allBotsDone
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Graceful shutdown failed")
		}

		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("Server exited with error")
		} else {
			logger.Info().Msg("Server shutdown complete")
		}
	case <-gameCompleteNotifier:
		logger.Info().Msg("Default game completed, waiting for bot and shutting down...")

		if cli.PrintStatsOnExit {
			printStats(toHTTPBase(cli.Addr))
		}

		// Wait for bot processes to exit before shutting down the server
		if allBotsDone != nil {
			logger.Info().Msg("Waiting for bot processes to exit...")
			<-allBotsDone
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Graceful shutdown failed")
		}

		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("Server exited with error")
		} else {
			logger.Info().Msg("Server shutdown complete")
		}
	case err := <-anyBotDone:
		logger.Info().Err(err).Msg("One or more bot processes exited, shutting down server...")

		if cli.PrintStatsOnExit {
			printStats(toHTTPBase(cli.Addr))
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Graceful shutdown failed")
		}

		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("Server exited with error")
		} else {
			logger.Info().Msg("Server shutdown complete")
		}
	}
}

// toWSURL converts server listen addr (e.g. ":8080" or "0.0.0.0:8080") to a ws URL.
func toWSURL(addr string) string {
	base := addr
	if strings.HasPrefix(base, ":") {
		base = "localhost" + base
	}
	if strings.HasPrefix(base, "0.0.0.0:") || strings.HasPrefix(base, "[::]:") {
		parts := strings.Split(base, ":")
		port := parts[len(parts)-1]
		base = "localhost:" + port
	}
	return "ws://" + base + "/ws"
}

func toHTTPBase(addr string) string {
	base := addr
	if strings.HasPrefix(base, ":") {
		base = "localhost" + base
	}
	if strings.HasPrefix(base, "0.0.0.0:") || strings.HasPrefix(base, "[::]:") {
		parts := strings.Split(base, ":")
		port := parts[len(parts)-1]
		base = "localhost:" + port
	}
	return "http://" + base
}

func spawnBot(logger zerolog.Logger, ctx context.Context, cmdStr, serverWS, gameID string) <-chan error {
	return spawnBotWithRole(logger, ctx, cmdStr, serverWS, gameID, "")
}

var externalBotSeq int64

func spawnBotWithRole(logger zerolog.Logger, ctx context.Context, cmdStr, serverWS, gameID, role string) <-chan error {
	logger.Info().Str("cmd", cmdStr).Str("server", serverWS).Str("game", gameID).Str("role", role).Msg("Spawning bot")
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	env := append(os.Environ(),
		"POKERFORBOTS_SERVER="+serverWS,
		"POKERFORBOTS_GAME="+gameID,
	)
	if role != "" {
		env = append(env, "POKERFORBOTS_ROLE="+role)
	}
	cmd.Env = env

	// Prefixed output
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	seq := atomic.AddInt64(&externalBotSeq, 1)
	base := strings.Fields(cmdStr)
	label := role
	if label == "" {
		label = "player"
	}
	cmdName := ""
	if len(base) > 0 {
		cmdName = filepath.Base(base[0])
	}
	prefix := fmt.Sprintf("[%s#%d %s] ", label, seq, cmdName)

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		logger.Error().Err(err).Str("cmd", cmdStr).Msg("Failed to start bot process")
		done <- err
		close(done)
		return done
	}
	go copyWithPrefix(os.Stdout, stdout, prefix)
	go copyWithPrefix(os.Stderr, stderr, prefix)
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Error().Err(err).Str("cmd", cmdStr).Msg("Bot process exited with error")
			done <- err
		} else {
			logger.Info().Str("cmd", cmdStr).Msg("Bot process exited")
			done <- nil
		}
		close(done)
	}()
	return done
}

func copyWithPrefix(dst *os.File, src io.Reader, prefix string) {
	s := bufio.NewScanner(src)
	for s.Scan() {
		fmt.Fprintln(dst, prefix+s.Text())
	}
}

func firstDone(chans []<-chan error) <-chan error {
	if len(chans) == 0 {
		return nil
	}
	out := make(chan error, 1)
	for _, ch := range chans {
		c := ch
		go func() {
			if err := <-c; err != nil {
				out <- err
			} else {
				out <- nil
			}
		}()
	}
	return out
}

func waitAll(chans []<-chan error) <-chan error {
	if len(chans) == 0 {
		return nil
	}
	out := make(chan error, 1)
	go func() {
		var firstErr error
		for _, ch := range chans {
			if err := <-ch; err != nil && firstErr == nil {
				firstErr = err
			}
		}
		out <- firstErr
		close(out)
	}()
	return out
}

func printStats(httpBase string) {
	// Prefer markdown format when available
	urlMD := fmt.Sprintf("%s/admin/games/default/stats.md", httpBase)
	resp, err := http.Get(urlMD)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		_, _ = io.Copy(os.Stdout, resp.Body)
		return
	}
	if resp != nil {
		resp.Body.Close()
	}
	// Fallback to pretty text
	urlTxt := fmt.Sprintf("%s/admin/games/default/stats.txt", httpBase)
	resp, err = http.Get(urlTxt)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		_, _ = io.Copy(os.Stdout, resp.Body)
		return
	}
	if resp != nil {
		resp.Body.Close()
	}
	// Fallback to JSON
	urlJSON := fmt.Sprintf("%s/admin/games/default/stats", httpBase)
	resp, err = http.Get(urlJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch stats: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "stats request failed: %s\n", resp.Status)
		return
	}
	_, _ = io.Copy(os.Stdout, resp.Body)
	fmt.Fprintln(os.Stdout)
}

func computeDefaultNPCSpecs(total, calling, random, aggro int) []server.NPCSpec {
	if total <= 0 && calling == 0 && random == 0 && aggro == 0 {
		return nil
	}

	if total > 0 && calling == 0 && random == 0 && aggro == 0 {
		base := total / 3
		remainder := total % 3
		calling = base
		random = base
		aggro = base
		if remainder >= 1 {
			calling++
		}
		if remainder >= 2 {
			random++
		}
	}

	specs := make([]server.NPCSpec, 0, 3)
	if calling > 0 {
		specs = append(specs, server.NPCSpec{Strategy: "calling", Count: calling})
	}
	if random > 0 {
		specs = append(specs, server.NPCSpec{Strategy: "random", Count: random})
	}
	if aggro > 0 {
		specs = append(specs, server.NPCSpec{Strategy: "aggressive", Count: aggro})
	}

	if len(specs) == 0 {
		return nil
	}
	return specs
}
