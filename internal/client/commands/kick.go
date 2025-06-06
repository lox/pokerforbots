package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lox/pokerforbots/internal/server"
)

// KickBotCommand kicks a bot from a table
type KickBotCommand struct {
	Table string `arg:"" name:"table" help:"Table ID to kick bot from"`
	Bot   string `arg:"" name:"bot" help:"Bot name to kick"`
}

func (cmd *KickBotCommand) Run(flags *GlobalFlags) error {
	wsClient, _, _, err := SetupClient(flags)
	if err != nil {
		return err
	}
	defer func() { _ = wsClient.Disconnect() }()

	// Set up response handler
	responseChan := make(chan bool, 1)
	wsClient.AddEventHandler("error", func(msg *server.Message) {
		var data server.ErrorData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			fmt.Printf("Error parsing error message: %v\n", err)
		} else {
			fmt.Printf("Error: %s\n", data.Message)
		}
		responseChan <- false
	})

	wsClient.AddEventHandler("bot_kicked", func(msg *server.Message) {
		fmt.Printf("Successfully kicked bot %s from table %s\n", cmd.Bot, cmd.Table)
		responseChan <- true
	})

	// Kick bot
	err = wsClient.KickBot(cmd.Table, cmd.Bot)
	if err != nil {
		return fmt.Errorf("failed to kick bot: %w", err)
	}

	// Wait for response
	select {
	case success := <-responseChan:
		if !success {
			return fmt.Errorf("failed to kick bot")
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for response")
	}
}
