package main

import (
	"github.com/alecthomas/kong"
)

type CLI struct {
	Server     ServerCmd     `cmd:"" help:"Run the poker server"`
	Client     ClientCmd     `cmd:"" help:"Connect as an interactive client"`
	Spawn      SpawnCmd      `cmd:"" help:"Spawn server with bots for testing/demos"`
	Regression RegressionCmd `cmd:"" help:"Run regression tests between bot versions"`
	Bots       BotsCmd       `cmd:"" help:"Run built-in example bots"`
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
	)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
