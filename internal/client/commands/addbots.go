package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lox/pokerforbots/internal/server"
)

// AddBotsCommand adds bots to a table
type AddBotsCommand struct {
	Table string `arg:"" name:"table" help:"Table ID to add bots to"`
	Count int    `short:"n" long:"count" default:"1" help:"Number of bots to add (1-5)"`
}

func (cmd *AddBotsCommand) Run(flags *GlobalFlags) error {
	if cmd.Count < 1 || cmd.Count > 5 {
		return fmt.Errorf("bot count must be between 1 and 5")
	}

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

	wsClient.AddEventHandler("bot_added", func(msg *server.Message) {
		fmt.Printf("Successfully added %d bot(s) to table %s\n", cmd.Count, cmd.Table)
		responseChan <- true
	})

	// Add bots
	err = wsClient.AddBots(cmd.Table, cmd.Count)
	if err != nil {
		return fmt.Errorf("failed to add bots: %w", err)
	}

	// Wait for response
	select {
	case success := <-responseChan:
		if !success {
			return fmt.Errorf("failed to add bots")
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for response")
	}
}
