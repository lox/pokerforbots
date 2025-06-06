package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/client/commands"
)

var CLI struct {
	commands.GlobalFlags

	JoinTable  commands.JoinTableCommand  `cmd:"join-table" help:"Join a table and start the interactive TUI (default command)" aliases:"join"`
	ListTables commands.ListTablesCommand `cmd:"list-tables" help:"List available tables" aliases:"join"`
	AddBots    commands.AddBotsCommand    `cmd:"add-bots" help:"Add bots to a table"`
	KickBot    commands.KickBotCommand    `cmd:"kick-bot" help:"Kick a bot from a table"`
}

func main() {
	ctx := kong.Parse(&CLI)

	// Execute the selected command
	err := ctx.Run(&CLI.GlobalFlags)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		ctx.Exit(1)
	}
}
