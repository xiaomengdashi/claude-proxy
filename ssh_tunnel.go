package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHTunnel manages SSH reverse tunnel connection
type SSHTunnel struct {
	config          *Config
	client          *ssh.Client
	listener        net.Listener
	mu              sync.Mutex
	stopChan        chan struct{}
	reconnect       bool
	OnStatusChange  func(connected bool, err error)
	PasswordPrompt  func() string // Callback to prompt for password if needed
}

// NewSSHTunnel creates a new SSH tunnel instance
func NewSSHTunnel(config *Config) *SSHTunnel {
	return &SSHTunnel{
		config:    config,
		stopChan:  make(chan struct{}),
		reconnect: true,
	}
}

// Start starts the SSH tunnel with auto-reconnection
func (t *SSHTunnel) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.stopChan:
			return nil
		default:
		}

		err := t.connect(ctx)
		if err != nil {
			fmt.Printf("[SSH] Connection error: %v\n", err)
			if t.OnStatusChange != nil {
				t.OnStatusChange(false, err)
			}
		}

		t.mu.Lock()
		shouldReconnect := t.reconnect
		t.mu.Unlock()

		if !shouldReconnect {
			return err
		}

		fmt.Println("[SSH] Reconnecting in 5 seconds...")
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		case <-t.stopChan:
			return nil
		}
	}
}

// getDefaultKeyPaths returns common SSH key paths
func getDefaultKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	sshDir := filepath.Join(home, ".ssh")
	return []string{
		filepath.Join(sshDir, "id_rsa"),
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_ecdsa"),
		filepath.Join(sshDir, "id_dsa"),
	}
}

// trySSHAgent tries to get auth from SSH agent
func (t *SSHTunnel) trySSHAgent() (ssh.AuthMethod, net.Conn) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, nil
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		fmt.Printf("[SSH] Could not connect to SSH agent: %v\n", err)
		return nil, nil
	}

	agentClient := agent.NewClient(conn)

	// Check if agent has any keys
	keys, err := agentClient.List()
	if err != nil {
		fmt.Printf("[SSH] Could not list agent keys: %v\n", err)
		conn.Close()
		return nil, nil
	}

	if len(keys) == 0 {
		fmt.Println("[SSH] SSH agent has no keys")
		conn.Close()
		return nil, nil
	}

	fmt.Printf("[SSH] SSH agent has %d key(s)\n", len(keys))
	for _, key := range keys {
		fmt.Printf("[SSH]   - %s\n", key.Comment)
	}

	return ssh.PublicKeysCallback(agentClient.Signers), conn
}

// buildAuthMethods builds a list of auth methods to try
func (t *SSHTunnel) buildAuthMethods() []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	// 1. Try SSH agent first (most seamless)
	if agentAuth, conn := t.trySSHAgent(); agentAuth != nil {
		_ = conn // Keep connection open
		methods = append(methods, agentAuth)
	}

	// 2. Try specified key path
	if t.config.SSHKeyPath != "" {
		if keyAuth, err := t.loadPrivateKeyFromPath(t.config.SSHKeyPath); err == nil {
			fmt.Printf("[SSH] Loaded key from %s\n", t.config.SSHKeyPath)
			methods = append(methods, keyAuth)
		} else {
			fmt.Printf("[SSH] Warning: Could not load key %s: %v\n", t.config.SSHKeyPath, err)
		}
	}

	// 3. Try default key paths
	for _, keyPath := range getDefaultKeyPaths() {
		if _, err := os.Stat(keyPath); err == nil {
			if keyAuth, err := t.loadPrivateKeyFromPath(keyPath); err == nil {
				fmt.Printf("[SSH] Found key at %s\n", keyPath)
				methods = append(methods, keyAuth)
			}
		}
	}

	// 4. Password auth (if provided or will be prompted)
	if t.config.SSHPassword != "" {
		methods = append(methods, ssh.Password(t.config.SSHPassword))
	}

	// 5. Keyboard-interactive for password prompt
	methods = append(methods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range questions {
			if t.config.SSHPassword != "" {
				answers[i] = t.config.SSHPassword
			} else if t.PasswordPrompt != nil {
				fmt.Printf("[SSH] Password required for %s@%s\n", t.config.SSHUser, t.config.SSHHost)
				answers[i] = t.PasswordPrompt()
			}
		}
		return answers, nil
	}))

	return methods
}

// connect establishes SSH connection and sets up reverse tunnel
func (t *SSHTunnel) connect(ctx context.Context) error {
	// Build auth methods (try keys first, then password)
	authMethods := t.buildAuthMethods()

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication methods available")
	}

	// SSH client configuration
	sshConfig := &ssh.ClientConfig{
		User:            t.config.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For simplicity; consider using known_hosts in production
		Timeout:         30 * time.Second,
	}

	// Connect to SSH server
	addr := fmt.Sprintf("%s:%d", t.config.SSHHost, t.config.SSHPort)
	fmt.Printf("[SSH] Dialing %s...\n", addr)

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		// If key auth failed, try prompting for password
		if t.config.SSHPassword == "" && t.PasswordPrompt != nil {
			fmt.Println("[SSH] Key authentication failed, prompting for password...")
			t.config.SSHPassword = t.PasswordPrompt()

			// Retry with password
			sshConfig.Auth = []ssh.AuthMethod{ssh.Password(t.config.SSHPassword)}
			client, err = ssh.Dial("tcp", addr, sshConfig)
		}
		if err != nil {
			return fmt.Errorf("failed to dial SSH: %w", err)
		}
	}

	t.mu.Lock()
	t.client = client
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.client = nil
		t.mu.Unlock()
		client.Close()
	}()

	fmt.Println("[SSH] Connected successfully")

	// Start reverse port forwarding
	// This makes B computer's localhost:RemotePort forward to A computer's localhost:ProxyPort
	remoteAddr := fmt.Sprintf("127.0.0.1:%d", t.config.RemotePort)
	listener, err := client.Listen("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on remote %s: %w", remoteAddr, err)
	}

	t.mu.Lock()
	t.listener = listener
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.listener = nil
		t.mu.Unlock()
		listener.Close()
	}()

	fmt.Printf("[SSH] Reverse tunnel established: B:%d -> A:%d\n", t.config.RemotePort, t.config.ProxyPort)

	// Notify successful connection
	if t.OnStatusChange != nil {
		t.OnStatusChange(true, nil)
	}

	// Start keepalive
	go t.keepalive(ctx, client)

	// Accept connections on the remote side
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.stopChan:
			return nil
		default:
		}

		remoteConn, err := listener.Accept()
		if err != nil {
			select {
			case <-t.stopChan:
				return nil
			default:
				return fmt.Errorf("accept error: %w", err)
			}
		}

		go t.handleRemoteConnection(remoteConn)
	}
}

// handleRemoteConnection forwards remote connection to local proxy
func (t *SSHTunnel) handleRemoteConnection(remoteConn net.Conn) {
	defer remoteConn.Close()

	// Connect to local proxy
	localAddr := fmt.Sprintf("127.0.0.1:%d", t.config.ProxyPort)
	localConn, err := net.DialTimeout("tcp", localAddr, 10*time.Second)
	if err != nil {
		fmt.Printf("[SSH] Failed to connect to local proxy: %v\n", err)
		return
	}
	defer localConn.Close()

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(localConn, remoteConn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(remoteConn, localConn)
	}()

	wg.Wait()
}

// keepalive sends periodic keepalive messages
func (t *SSHTunnel) keepalive(ctx context.Context, client *ssh.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		case <-ticker.C:
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				fmt.Printf("[SSH] Keepalive failed: %v\n", err)
				return
			}
		}
	}
}

// loadPrivateKeyFromPath loads SSH private key from specified path
func (t *SSHTunnel) loadPrivateKeyFromPath(keyPath string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		// Try with passphrase if provided
		if t.config.SSHKeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(t.config.SSHKeyPassphrase))
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return ssh.PublicKeys(signer), nil
}

// Stop stops the SSH tunnel
func (t *SSHTunnel) Stop() {
	t.mu.Lock()
	t.reconnect = false
	close(t.stopChan)

	if t.listener != nil {
		t.listener.Close()
	}
	if t.client != nil {
		t.client.Close()
	}
	t.mu.Unlock()

	fmt.Println("[SSH] Tunnel stopped")
}

// readFile reads a file and returns its contents
func readFile(path string) ([]byte, error) {
	return readFileFromPath(path)
}
