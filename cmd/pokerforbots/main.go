package main

import (
	"github.com/alecthomas/kong"
)

// version is set by ldflags during build
var version = "dev"

type CLI struct {
	Version     kong.VersionFlag `short:"v" help:"Show version"`
	Server      ServerCmd        `cmd:"" help:"Run the poker server"`
	Client      ClientCmd        `cmd:"" help:"Connect as an interactive client"`
	Bot         BotCmd           `cmd:"" help:"Run a built-in bot"`
	Spawn       SpawnCmd         `cmd:"" help:"Spawn server with bots for testing/demos"`
	Regression  RegressionCmd    `cmd:"" help:"Run regression tests between bot versions"`
	HandHistory HandHistoryCmd   `cmd:"hand-history" help:"Work with PHH hand history files"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("pokerforbots"),
		kong.Description("High-performance poker server for bot-vs-bot play"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": version,
		},
	)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
