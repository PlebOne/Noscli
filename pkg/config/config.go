package config

import (
"encoding/json"
"fmt"
"os"
"path/filepath"
)

// Config holds all persistent settings
type Config struct {
AuthMethod string   `json:"auth_method"` // "pleb_signer" or "nsec"
Nsec       string   `json:"nsec"`        // Encrypted or plaintext nsec (user's choice to save)
Relays     []string `json:"relays"`
NWC        string   `json:"nwc"` // Nostr Wallet Connect string
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
// Use XDG_CONFIG_HOME if set, otherwise ~/.config
configDir := os.Getenv("XDG_CONFIG_HOME")
if configDir == "" {
home, err := os.UserHomeDir()
if err != nil {
return "", fmt.Errorf("failed to get home directory: %w", err)
}
configDir = filepath.Join(home, ".config")
}

noscliDir := filepath.Join(configDir, "noscli")

// Create directory if it doesn't exist
if err := os.MkdirAll(noscliDir, 0700); err != nil {
return "", fmt.Errorf("failed to create config directory: %w", err)
}

return filepath.Join(noscliDir, "config.json"), nil
}

// Load reads the config file
func Load() (*Config, error) {
path, err := GetConfigPath()
if err != nil {
return nil, err
}

// If file doesn't exist, return empty config
if _, err := os.Stat(path); os.IsNotExist(err) {
return &Config{
Relays: []string{"wss://relay.damus.io", "wss://relay.nostr.band"},
}, nil
}

data, err := os.ReadFile(path)
if err != nil {
return nil, fmt.Errorf("failed to read config: %w", err)
}

var cfg Config
if err := json.Unmarshal(data, &cfg); err != nil {
return nil, fmt.Errorf("failed to parse config: %w", err)
}

// Ensure we have default relays if none saved
if len(cfg.Relays) == 0 {
cfg.Relays = []string{"wss://relay.damus.io", "wss://relay.nostr.band"}
}

return &cfg, nil
}

// Save writes the config file
func Save(cfg *Config) error {
path, err := GetConfigPath()
if err != nil {
return err
}

data, err := json.MarshalIndent(cfg, "", "  ")
if err != nil {
return fmt.Errorf("failed to marshal config: %w", err)
}

// Write with restricted permissions (user only)
if err := os.WriteFile(path, data, 0600); err != nil {
return fmt.Errorf("failed to write config: %w", err)
}

return nil
}

// Clear deletes the config file
func Clear() error {
path, err := GetConfigPath()
if err != nil {
return err
}

if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
return fmt.Errorf("failed to remove config: %w", err)
}

return nil
}
