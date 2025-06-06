package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lox/pokerforbots/internal/server"
)

// ListTablesCommand lists all available tables
type ListTablesCommand struct {
}

func (cmd *ListTablesCommand) Run(flags *GlobalFlags) error {
	wsClient, _, _, err := SetupClient(flags)
	if err != nil {
		return err
	}
	defer func() { _ = wsClient.Disconnect() }()

	// Set up a channel to capture table list responses
	responseChan := make(chan bool, 1)

	wsClient.AddEventHandler("table_list", func(msg *server.Message) {
		var data server.TableListData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			fmt.Printf("Error parsing table list: %v\n", err)
			responseChan <- false
			return
		}

		// Print table information
		if len(data.Tables) == 0 {
			fmt.Println("No tables available")
		} else {
			fmt.Printf("Available tables:\n")
			for _, table := range data.Tables {
				fmt.Printf("  %s: %d/%d players, stakes %s\n",
					table.ID, table.PlayerCount, table.MaxPlayers,
					table.Stakes)
			}
		}
		responseChan <- true
	})

	// Request table list
	err = wsClient.ListTables()
	if err != nil {
		return fmt.Errorf("failed to request table list: %w", err)
	}

	// Wait for response with timeout
	select {
	case <-responseChan:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for table list response")
	}
}
