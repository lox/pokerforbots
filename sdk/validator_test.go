package sdk

import (
	"encoding/json"
	"testing"
)

func TestValidator(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test valid auth message
	authMsg := map[string]interface{}{
		"type": "auth",
		"data": map[string]interface{}{
			"playerName": "TestBot",
		},
	}
	
	authData, _ := json.Marshal(authMsg)
	if err := validator.ValidateMessage(authData); err != nil {
		t.Errorf("Valid auth message failed validation: %v", err)
	}

	// Test invalid auth message (missing playerName)
	invalidAuthMsg := map[string]interface{}{
		"type": "auth",
		"data": map[string]interface{}{},
	}
	
	invalidAuthData, _ := json.Marshal(invalidAuthMsg)
	if err := validator.ValidateMessage(invalidAuthData); err == nil {
		t.Error("Invalid auth message should have failed validation")
	}

	// Test valid player decision message
	decisionMsg := map[string]interface{}{
		"type": "player_decision",
		"data": map[string]interface{}{
			"tableId": "table1",
			"action":  "call",
			"amount":  10,
		},
	}
	
	decisionData, _ := json.Marshal(decisionMsg)
	if err := validator.ValidateMessage(decisionData); err != nil {
		t.Errorf("Valid decision message failed validation: %v", err)
	}

	// Test invalid action
	invalidDecisionMsg := map[string]interface{}{
		"type": "player_decision",
		"data": map[string]interface{}{
			"tableId": "table1",
			"action":  "invalid_action",
		},
	}
	
	invalidDecisionData, _ := json.Marshal(invalidDecisionMsg)
	if err := validator.ValidateMessage(invalidDecisionData); err == nil {
		t.Error("Invalid action should have failed validation")
	}
}

func TestAvailableSchemas(t *testing.T) {
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	schemas := validator.GetAvailableSchemas()
	expectedSchemas := []string{"message", "error", "auth", "game", "table"}
	
	if len(schemas) != len(expectedSchemas) {
		t.Errorf("Expected %d schemas, got %d", len(expectedSchemas), len(schemas))
	}

	schemaMap := make(map[string]bool)
	for _, schema := range schemas {
		schemaMap[schema] = true
	}

	for _, expected := range expectedSchemas {
		if !schemaMap[expected] {
			t.Errorf("Missing expected schema: %s", expected)
		}
	}
}
