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

	// Return a copy without password
	config := *a.config
	config.SSHPassword = ""
	config.SSHKeyPassphrase = ""
	return &config
}

// SaveConfig saves the configuration
func (a *App) SaveConfig(config *Config) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()

	// Apply defaults
	if config.SSHPort == 0 {
		config.SSHPort = 22
	}
	if config.ProxyPort == 0 {
		config.ProxyPort = 8080
	}
	if config.RemotePort == 0 {
		config.RemotePort = 8080
	}

	*a.config = *config

	// Save to file (without password)
	if err := config.Save(); err != nil {
		a.addLog(fmt.Sprintf("保存配置失败: %v", err))
		return err
	}

	a.addLog("配置已保存")
	return nil
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

	// Update stored config
	a.configMu.Lock()
	*a.config = *config
	a.configMu.Unlock()

	// Save config (without password)
	config.Save()

	// Create new context
	a.tunnelCtx, a.tunnelStop = context.WithCancel(context.Background())

	// Update status
	a.updateStatus(false, false, true, "")
	a.addLog("正在启动代理服务器...")

	// Start proxy server
	a.proxy = NewProxyServer(config.ProxyPort)
	go func() {
		a.addLog(fmt.Sprintf("代理服务监听 127.0.0.1:%d", config.ProxyPort))
		a.updateStatus(true, false, true, "")
		if err := a.proxy.Start(); err != nil {
			a.addLog(fmt.Sprintf("代理服务错误: %v", err))
			a.updateStatus(false, false, false, err.Error())
		}
	}()

	// Start SSH tunnel
	a.tunnel = NewSSHTunnel(config)
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

// addLog adds a log message
func (a *App) addLog(msg string) {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", timestamp, msg)
	a.logs = append(a.logs, logEntry)

	// Keep only last 100 logs
	if len(a.logs) > 100 {
		a.logs = a.logs[len(a.logs)-100:]
	}
}
