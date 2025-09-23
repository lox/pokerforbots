package config

import (
	"os"
	"testing"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *BotConfig
		wantErr bool
	}{
		{
			name: "all variables set",
			env: map[string]string{
				EnvServer: "ws://localhost:8080/ws",
				EnvSeed:   "12345",
				EnvBotID:  "bot-1",
				EnvGame:   "tournament",
			},
			want: &BotConfig{
				ServerURL: "ws://localhost:8080/ws",
				Seed:      12345,
				BotID:     "bot-1",
				GameID:    "tournament",
			},
		},
		{
			name: "only required variables",
			env: map[string]string{
				EnvServer: "ws://localhost:8080/ws",
			},
			want: &BotConfig{
				ServerURL: "ws://localhost:8080/ws",
				GameID:    "default",
			},
		},
		{
			name:    "missing server URL",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name: "invalid seed",
			env: map[string]string{
				EnvServer: "ws://localhost:8080/ws",
				EnvSeed:   "not-a-number",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			got, err := FromEnv()
			if (err != nil) != tt.wantErr {
				t.Errorf("FromEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// Compare configs
			if got.ServerURL != tt.want.ServerURL {
				t.Errorf("ServerURL = %v, want %v", got.ServerURL, tt.want.ServerURL)
			}
			if got.Seed != tt.want.Seed {
				t.Errorf("Seed = %v, want %v", got.Seed, tt.want.Seed)
			}
			if got.BotID != tt.want.BotID {
				t.Errorf("BotID = %v, want %v", got.BotID, tt.want.BotID)
			}
			if got.GameID != tt.want.GameID {
				t.Errorf("GameID = %v, want %v", got.GameID, tt.want.GameID)
			}
		})
	}
}

func TestSetEnv(t *testing.T) {
	env := []string{"EXISTING=value"}
	env = SetEnv(env, "NEW_KEY", "new_value")

	if len(env) != 2 {
		t.Errorf("Expected 2 environment variables, got %d", len(env))
	}
	if env[1] != "NEW_KEY=new_value" {
		t.Errorf("Expected 'NEW_KEY=new_value', got %s", env[1])
	}
}
