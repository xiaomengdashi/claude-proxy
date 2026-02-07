package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// App struct holds the application state
type App struct {
	ctx          context.Context
	config       *Config
	configMu     sync.RWMutex
	status       *Status
	statusMu     sync.RWMutex
	logs         []string
	logsMu       sync.RWMutex
	proxy        *ProxyServer
	tunnel       *SSHTunnel
	tunnelCtx    context.Context
	tunnelStop   context.CancelFunc
	mu           sync.Mutex
}

// Status holds the current connection status
type Status struct {
	ProxyRunning    bool   `json:"proxy_running"`
	TunnelConnected bool   `json:"tunnel_connected"`
	TunnelRunning   bool   `json:"tunnel_running"`
	LastError       string `json:"last_error,omitempty"`
	StartTime       string `json:"start_time,omitempty"`
}

type RecordsResponse struct {
	Records  []RemoteRecord `json:"records"`
	ActiveID string         `json:"active_id"`
}

// NewApp creates a new App instance
func NewApp() *App {
	return &App{
		status: &Status{},
		logs:   make([]string, 0, 100),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load configuration
	config, err := LoadOrCreateConfig()
	if err != nil {
		a.addLog(fmt.Sprintf("配置加载错误: %v", err))
		config = DefaultConfig()
	}
	a.config = config
	a.addLog("应用启动完成")
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	a.Stop()
}

// GetConfig returns the current configuration (without sensitive data)
func (a *App) GetConfig() *Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()

	config := *a.config
	config.SSHPassword = ""
	config.SSHKeyPassphrase = ""
	config.syncLegacyFromActive()
	return &config
}

// SaveConfig saves the configuration
func (a *App) SaveConfig(config *Config) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	if config.SSHPort == 0 {
		config.SSHPort = 22
	}
	if config.ProxyPort == 0 {
		config.ProxyPort = 8080
	}
	if config.RemotePort == 0 {
		config.RemotePort = 8080
	}

	a.upsertRecordFromConfigLocked(config)
	if err := a.config.Save(); err != nil {
		a.addLog(fmt.Sprintf("保存配置失败: %v", err))
		return err
	}

	a.addLog("配置已保存")
	return nil
}

func (a *App) GetRecords() *RecordsResponse {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.recordsResponseLocked()
}

func (a *App) SaveRecord(record *RemoteRecord) (*RecordsResponse, error) {
	if record == nil {
		return nil, fmt.Errorf("record is nil")
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.config.normalize()
	newRecord := *record
	if newRecord.ID == "" {
		newRecord.ID = newRecordID()
	}
	if newRecord.Name == "" {
		if idx := a.config.findRecordIndex(newRecord.ID); idx >= 0 {
			newRecord.Name = a.config.Records[idx].Name
		}
	}
	a.config.applyDefaultsToRecord(&newRecord)

	if idx := a.config.findRecordIndex(newRecord.ID); idx >= 0 {
		a.config.Records[idx] = newRecord
	} else {
		a.config.Records = append(a.config.Records, newRecord)
	}
	a.config.ActiveID = newRecord.ID
	a.config.syncLegacyFromActive()

	if err := a.config.Save(); err != nil {
		a.addLog(fmt.Sprintf("保存配置失败: %v", err))
		return nil, err
	}

	return a.recordsResponseLocked(), nil
}

func (a *App) DeleteRecord(id string) (*RecordsResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("record id is empty")
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.config.normalize()
	if idx := a.config.findRecordIndex(id); idx >= 0 {
		a.config.Records = append(a.config.Records[:idx], a.config.Records[idx+1:]...)
	}

	if len(a.config.Records) == 0 {
		defaultConfig := DefaultConfig()
		a.config.Records = defaultConfig.Records
		a.config.ActiveID = defaultConfig.ActiveID
	}

	if a.config.findRecordIndex(a.config.ActiveID) == -1 && len(a.config.Records) > 0 {
		a.config.ActiveID = a.config.Records[0].ID
	}

	a.config.syncLegacyFromActive()
	if err := a.config.Save(); err != nil {
		a.addLog(fmt.Sprintf("保存配置失败: %v", err))
		return nil, err
	}

	return a.recordsResponseLocked(), nil
}

func (a *App) SetActiveRecord(id string) (*Config, error) {
	if id == "" {
		return nil, fmt.Errorf("record id is empty")
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()

	a.config.normalize()
	if a.config.findRecordIndex(id) == -1 {
		return nil, fmt.Errorf("record not found")
	}
	a.config.ActiveID = id
	a.config.syncLegacyFromActive()
	if err := a.config.Save(); err != nil {
		a.addLog(fmt.Sprintf("保存配置失败: %v", err))
		return nil, err
	}

	config := *a.config
	config.SSHPassword = ""
	config.SSHKeyPassphrase = ""
	config.syncLegacyFromActive()
	return &config, nil
}

// GetStatus returns the current status
func (a *App) GetStatus() *Status {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()

	status := *a.status
	return &status
}

// GetLogs returns the recent logs
func (a *App) GetLogs() []string {
	a.logsMu.RLock()
	defer a.logsMu.RUnlock()

	logs := make([]string, len(a.logs))
	copy(logs, a.logs)
	return logs
}

// Start starts the proxy and SSH tunnel
func (a *App) Start(config *Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Stop existing tunnel if running
	if a.tunnelStop != nil {
		a.tunnelStop()
	}
	if a.tunnel != nil {
		a.tunnel.Stop()
	}
	if a.proxy != nil {
		a.proxy.Stop()
	}

	if config.SSHPort == 0 {
		config.SSHPort = 22
	}
	if config.ProxyPort == 0 {
		config.ProxyPort = 8080
	}
	if config.RemotePort == 0 {
		config.RemotePort = 8080
	}

	a.configMu.Lock()
	a.upsertRecordFromConfigLocked(config)
	a.configMu.Unlock()

	a.config.Save()

	// Create new context
	a.tunnelCtx, a.tunnelStop = context.WithCancel(context.Background())

	// Update status
	a.updateStatus(false, false, true, "")
	a.addLog("正在启动代理服务器...")

	// Start proxy server
	a.proxy = NewProxyServer(config.ProxyPort, config.HTTPProxy, config.HTTPSProxy, func(level, msg string) {
		a.log(level, msg)
	})
	go func() {
		if config.HTTPProxy != "" || config.HTTPSProxy != "" {
			a.addLog(fmt.Sprintf("上游代理: HTTP=%s HTTPS=%s", config.HTTPProxy, config.HTTPSProxy))
		}
		a.addLog(fmt.Sprintf("代理服务监听 127.0.0.1:%d", config.ProxyPort))
		a.updateStatus(true, false, true, "")
		if err := a.proxy.Start(); err != nil {
			a.addLog(fmt.Sprintf("代理服务错误: %v", err))
			a.updateStatus(false, false, false, err.Error())
		}
	}()

	// Start SSH tunnel
	a.tunnel = NewSSHTunnel(config, func(level, msg string) {
		a.log(level, msg)
	})
	a.tunnel.OnStatusChange = func(connected bool, err error) {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
			a.addLog(fmt.Sprintf("SSH 状态: %v", err))
		}
		if connected {
			a.addLog("SSH 隧道连接成功!")
		}
		a.updateStatus(true, connected, true, errMsg)
	}

	go func() {
		a.addLog(fmt.Sprintf("正在连接 %s@%s:%d...", config.SSHUser, config.SSHHost, config.SSHPort))

		if err := a.tunnel.Start(a.tunnelCtx); err != nil {
			a.addLog(fmt.Sprintf("SSH 隧道错误: %v", err))
			a.updateStatus(true, false, false, err.Error())
		}
	}()

	return nil
}

// Stop stops the proxy and SSH tunnel
func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.tunnelStop != nil {
		a.tunnelStop()
		a.tunnelStop = nil
	}

	if a.tunnel != nil {
		a.tunnel.Stop()
		a.tunnel = nil
	}

	if a.proxy != nil {
		a.proxy.Stop()
		a.proxy = nil
	}

	a.updateStatus(false, false, false, "")
	a.addLog("隧道已停止")
}

// updateStatus updates the connection status
func (a *App) updateStatus(proxyRunning, tunnelConnected, tunnelRunning bool, lastError string) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.status.ProxyRunning = proxyRunning
	a.status.TunnelConnected = tunnelConnected
	a.status.TunnelRunning = tunnelRunning
	a.status.LastError = lastError
}

const (
	LevelError = "ERROR"
	LevelInfo  = "INFO"
	LevelDebug = "DEBUG"
)

// LogFunc is a function for logging
type LogFunc func(level, msg string)

// log adds a log message with level check
func (a *App) log(level, msg string) {
	a.configMu.RLock()
	configLevel := a.config.LogLevel
	if configLevel == "" {
		configLevel = LevelInfo
	}
	a.configMu.RUnlock()

	// Filter logs
	shouldLog := false
	switch configLevel {
	case LevelError:
		shouldLog = level == LevelError
	case LevelInfo:
		shouldLog = level == LevelError || level == LevelInfo
	case LevelDebug:
		shouldLog = true
	default:
		shouldLog = level == LevelError || level == LevelInfo
	}

	if !shouldLog {
		return
	}

	a.logsMu.Lock()
	defer a.logsMu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	
	// Add prefix for Error and Debug
	prefix := ""
	if level == LevelError {
		prefix = "[ERROR] "
	} else if level == LevelDebug {
		prefix = "[DEBUG] "
	}

	logEntry := fmt.Sprintf("[%s] %s%s", timestamp, prefix, msg)
	a.logs = append(a.logs, logEntry)

	// Keep only last 100 logs
	if len(a.logs) > 100 {
		a.logs = a.logs[len(a.logs)-100:]
	}
}

// addLog adds a default INFO log message (for backward compatibility)
func (a *App) addLog(msg string) {
	a.log(LevelInfo, msg)
}

// ClearLogs clears the logs
func (a *App) ClearLogs() {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()
	a.logs = make([]string, 0, 100)
}

func (a *App) recordsResponseLocked() *RecordsResponse {
	records := make([]RemoteRecord, len(a.config.Records))
	copy(records, a.config.Records)
	return &RecordsResponse{
		Records:  records,
		ActiveID: a.config.ActiveID,
	}
}

func (a *App) upsertRecordFromConfigLocked(config *Config) {
	a.config.normalize()
	recordID := config.ActiveID
	if recordID == "" {
		recordID = newRecordID()
	}
	record := RemoteRecord{
		ID:         recordID,
		Name:       config.RecordName,
		SSHHost:    config.SSHHost,
		SSHPort:    config.SSHPort,
		SSHUser:    config.SSHUser,
		SSHKeyPath: config.SSHKeyPath,
		ProxyPort:  config.ProxyPort,
		RemotePort: config.RemotePort,
		HTTPProxy:  config.HTTPProxy,
		HTTPSProxy: config.HTTPSProxy,
		LogLevel:   config.LogLevel,
	}
	if record.Name == "" {
		if idx := a.config.findRecordIndex(record.ID); idx >= 0 {
			record.Name = a.config.Records[idx].Name
		}
	}
	a.config.applyDefaultsToRecord(&record)

	if idx := a.config.findRecordIndex(record.ID); idx >= 0 {
		a.config.Records[idx] = record
	} else {
		a.config.Records = append(a.config.Records, record)
	}
	a.config.ActiveID = record.ID
	a.config.syncLegacyFromActive()
}
