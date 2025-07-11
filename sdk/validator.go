package sdk

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schemas
var schemaFiles embed.FS

// Validator provides JSON schema validation for WebSocket messages
type Validator struct {
	compiler *jsonschema.Compiler
	schemas  map[string]*jsonschema.Schema
}

// NewValidator creates a new message validator with all schemas loaded
func NewValidator() (*Validator, error) {
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020

	// Load all schema files
	_, err := schemaFiles.ReadDir("schemas")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema directory: %w", err)
	}

	schemas := make(map[string]*jsonschema.Schema)
	
	// Load schemas in dependency order
	schemaOrder := []string{"message.json", "error.json", "auth.json", "game.json", "table.json"}
	
	for _, filename := range schemaOrder {
		data, err := schemaFiles.ReadFile("schemas/" + filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema %s: %w", filename, err)
		}

		// Parse schema name from filename
		schemaName := strings.TrimSuffix(filename, ".json")
		schemaURL := fmt.Sprintf("https://pokerforbots.com/schemas/%s", filename)

		// Add schema to compiler
		if err := compiler.AddResource(schemaURL, strings.NewReader(string(data))); err != nil {
			return nil, fmt.Errorf("failed to add schema %s: %w", filename, err)
		}

		// Compile schema
		schema, err := compiler.Compile(schemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to compile schema %s: %w", filename, err)
		}

		schemas[schemaName] = schema
	}

	return &Validator{
		compiler: compiler,
		schemas:  schemas,
	}, nil
}

// ValidateMessage validates a complete WebSocket message
func (v *Validator) ValidateMessage(messageData []byte) error {
	// First parse to get message type
	var msg struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	
	if err := json.Unmarshal(messageData, &msg); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate against base message schema
	if err := v.ValidateAgainstSchema("message", messageData); err != nil {
		return fmt.Errorf("message format validation failed: %w", err)
	}

	// Validate against specific message type schema
	switch {
	case msg.Type == "auth" || msg.Type == "auth_response":
		return v.ValidateAgainstSchema("auth", messageData)
	case msg.Type == "join_table" || msg.Type == "leave_table" || msg.Type == "table_joined":
		return v.ValidateAgainstSchema("table", messageData)
	case msg.Type == "action_required" || msg.Type == "player_decision":
		return v.ValidateAgainstSchema("game", messageData)
	case msg.Type == "error":
		return v.ValidateAgainstSchema("error", messageData)
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// ValidateAgainstSchema validates data against a specific schema
func (v *Validator) ValidateAgainstSchema(schemaName string, data []byte) error {
	schema, exists := v.schemas[schemaName]
	if !exists {
		return fmt.Errorf("schema not found: %s", schemaName)
	}

	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if err := schema.Validate(jsonData); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

// ValidateStruct validates a Go struct against a specific schema
func (v *Validator) ValidateStruct(schemaName string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal struct: %w", err)
	}
	
	return v.ValidateAgainstSchema(schemaName, jsonData)
}

// GetAvailableSchemas returns list of available schema names
func (v *Validator) GetAvailableSchemas() []string {
	schemas := make([]string, 0, len(v.schemas))
	for name := range v.schemas {
		schemas = append(schemas, name)
	}
	return schemas
}
