package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RemoteRecord struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	SSHHost    string `json:"ssh_host"`
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	SSHKeyPath string `json:"ssh_key_path,omitempty"`
	ProxyPort  int    `json:"proxy_port"`
	RemotePort int    `json:"remote_port"`
	HTTPProxy  string `json:"http_proxy,omitempty"`
	HTTPSProxy string `json:"https_proxy,omitempty"`
	LogLevel   string `json:"log_level,omitempty"` // DEBUG, INFO, ERROR
}

type Config struct {
	Records   []RemoteRecord `json:"records,omitempty"`
	ActiveID  string         `json:"active_id,omitempty"`
	RecordName string        `json:"record_name,omitempty"`

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

	// Upstream proxy settings (for A computer to access internet)
	// Upstream proxy settings (for A computer to access internet)
	HTTPProxy  string `json:"http_proxy,omitempty"`
	HTTPSProxy string `json:"https_proxy,omitempty"`
	
	// App settings
	LogLevel string `json:"log_level"` // DEBUG, INFO, ERROR
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	record := RemoteRecord{
		ID:         newRecordID(),
		SSHPort:    22,
		ProxyPort:  8080,
		RemotePort: 8080,
	}
	config := &Config{
		Records:  []RemoteRecord{record},
		ActiveID: record.ID,
	}
	config.syncLegacyFromActive()
	return config
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

	config.normalize()
	return &config, nil
}

// Save saves the configuration to file
func (c *Config) Save() error {
	configPath := ConfigPath()
	c.normalize()
	c.syncLegacyFromActive()

	// Don't save password in plain text - just save other settings
	configToSave := *c
	configToSave.SSHPassword = ""
	configToSave.SSHKeyPassphrase = ""
	configToSave.RecordName = ""

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

func newRecordID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (c *Config) normalize() {
	if len(c.Records) == 0 {
		record := RemoteRecord{
			ID:         newRecordID(),
			Name:       c.RecordName,
			SSHHost:    c.SSHHost,
			SSHPort:    c.SSHPort,
			SSHUser:    c.SSHUser,
			SSHKeyPath: c.SSHKeyPath,
			ProxyPort:  c.ProxyPort,
			RemotePort: c.RemotePort,
			HTTPProxy:  c.HTTPProxy,
			HTTPSProxy: c.HTTPSProxy,
		}
		c.applyDefaultsToRecord(&record)
		c.Records = []RemoteRecord{record}
	}

	for i := range c.Records {
		c.applyDefaultsToRecord(&c.Records[i])
		if c.Records[i].ID == "" {
			c.Records[i].ID = newRecordID()
		}
	}

	if c.ActiveID == "" && len(c.Records) > 0 {
		c.ActiveID = c.Records[0].ID
	}
	if c.ActiveID != "" && c.findRecordIndex(c.ActiveID) == -1 && len(c.Records) > 0 {
		c.ActiveID = c.Records[0].ID
	}
}

func (c *Config) applyDefaultsToRecord(record *RemoteRecord) {
	if record.SSHPort == 0 {
		record.SSHPort = 22
	}
	if record.ProxyPort == 0 {
		record.ProxyPort = 8080
	}
	if record.RemotePort == 0 {
		record.RemotePort = 8080
	}
}

func (c *Config) getActiveRecord() *RemoteRecord {
	if c.ActiveID == "" {
		return nil
	}
	for i := range c.Records {
		if c.Records[i].ID == c.ActiveID {
			return &c.Records[i]
		}
	}
	return nil
}

func (c *Config) findRecordIndex(id string) int {
	for i := range c.Records {
		if c.Records[i].ID == id {
			return i
		}
	}
	return -1
}

func (c *Config) syncLegacyFromActive() {
	record := c.getActiveRecord()
	if record == nil {
		return
	}
	c.RecordName = record.Name
	c.SSHHost = record.SSHHost
	c.SSHPort = record.SSHPort
	c.SSHUser = record.SSHUser
	c.SSHKeyPath = record.SSHKeyPath
	c.ProxyPort = record.ProxyPort
	c.RemotePort = record.RemotePort
	c.HTTPProxy = record.HTTPProxy
	c.HTTPSProxy = record.HTTPSProxy
	c.LogLevel = record.LogLevel
	if c.LogLevel == "" {
		c.LogLevel = "INFO"
	}
}
