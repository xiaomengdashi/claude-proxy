package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration
type Config struct {
	// SSH connection settings
	SSHHost          string `json:"ssh_host"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHPassword      string `json:"ssh_password,omitempty"`
	SSHKeyPath       string `json:"ssh_key_path,omitempty"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase,omitempty"`

	// Proxy settings
	ProxyPort  int `json:"proxy_port"`
	RemotePort int `json:"remote_port"`
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		SSHPort:    22,
		ProxyPort:  8080,
		RemotePort: 8080,
	}
}

// IsComplete checks if the configuration has all required fields
func (c *Config) IsComplete() bool {
	if c.SSHHost == "" || c.SSHUser == "" {
		return false
	}
	if c.SSHPassword == "" && c.SSHKeyPath == "" {
		return false
	}
	return true
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "claude-proxy.json"
	}
	return filepath.Join(homeDir, ".claude-proxy.json")
}

// LoadOrCreateConfig loads config from file or creates default
func LoadOrCreateConfig() (*Config, error) {
	configPath := ConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults for missing fields
	if config.SSHPort == 0 {
		config.SSHPort = 22
	}
	if config.ProxyPort == 0 {
		config.ProxyPort = 8080
	}
	if config.RemotePort == 0 {
		config.RemotePort = 8080
	}

	return &config, nil
}

// Save saves the configuration to file
func (c *Config) Save() error {
	configPath := ConfigPath()

	// Don't save password in plain text - just save other settings
	configToSave := *c
	configToSave.SSHPassword = ""
	configToSave.SSHKeyPassphrase = ""

	data, err := json.MarshalIndent(configToSave, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("[Config] Saved to %s\n", configPath)
	return nil
}

// readFileFromPath reads a file and returns its contents
func readFileFromPath(path string) ([]byte, error) {
	return os.ReadFile(path)
}
