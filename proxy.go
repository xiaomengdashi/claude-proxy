package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyServer handles HTTP CONNECT proxy requests
type ProxyServer struct {
	port        int
	server      *http.Server
	listener    net.Listener
	mu          sync.Mutex
	httpProxy   string
	httpsProxy  string
	proxyDialer *net.Dialer
	log         LogFunc
}

// NewProxyServer creates a new proxy server instance
func NewProxyServer(port int, httpProxy, httpsProxy string, log LogFunc) *ProxyServer {
	return &ProxyServer{
		port:        port,
		httpProxy:   httpProxy,
		httpsProxy:  httpsProxy,
		proxyDialer: &net.Dialer{Timeout: 30 * time.Second},
		log:         log,
	}
}

// Start starts the proxy server
func (p *ProxyServer) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", p.port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	p.mu.Lock()
	p.listener = listener
	p.server = &http.Server{
		Handler:      http.HandlerFunc(p.handleRequest),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // No write timeout for streaming
		IdleTimeout:  120 * time.Second,
	}
	p.mu.Unlock()

	p.log(LevelInfo, fmt.Sprintf("Listening on %s", addr))
	return p.server.Serve(listener)
}

// Stop stops the proxy server
func (p *ProxyServer) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.server != nil {
		p.server.Close()
	}
}

// handleRequest handles incoming proxy requests
func (p *ProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

// handleConnect handles HTTPS CONNECT tunnel requests
func (p *ProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	p.log(LevelDebug, fmt.Sprintf(">> 收到 HTTPS 请求: %s", r.Host))

	var targetConn net.Conn
	var err error

	// Check if we need to use upstream proxy for HTTPS
	if p.httpsProxy != "" {
		targetConn, err = p.dialThroughProxy(p.httpsProxy, r.Host)
	} else {
		// Direct connection
		targetConn, err = net.DialTimeout("tcp", r.Host, 30*time.Second)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to %s: %v", r.Host, err), http.StatusBadGateway)
		p.log(LevelError, fmt.Sprintf("Failed to connect to %s: %v", r.Host, err))
		return
	}
	defer targetConn.Close()

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("Hijack failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Send 200 Connection Established
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		p.log(LevelError, fmt.Sprintf("Failed to send connection established: %v", err))
		return
	}

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Target
	go func() {
		defer wg.Done()
		io.Copy(targetConn, clientConn)
		targetConn.(*net.TCPConn).CloseWrite()
	}()

	// Target -> Client
	go func() {
		defer wg.Done()
		io.Copy(clientConn, targetConn)
		clientConn.(*net.TCPConn).CloseWrite()
	}()

	wg.Wait()
	p.log(LevelDebug, fmt.Sprintf("CONNECT %s completed", r.Host))
}

// dialThroughProxy connects to target through an HTTP proxy
func (p *ProxyServer) dialThroughProxy(proxyURL, targetHost string) (net.Conn, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Connect to the proxy server
	proxyAddr := parsedURL.Host
	if parsedURL.Port() == "" {
		proxyAddr = net.JoinHostPort(parsedURL.Hostname(), "80")
	}

	conn, err := p.proxyDialer.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy %s: %w", proxyAddr, err)
	}

	// Send CONNECT request to proxy
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetHost, targetHost)
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send CONNECT to proxy: %w", err)
	}

	// Read proxy response
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read proxy response: %w", err)
	}

	// Check if proxy accepted the connection (200 or 201)
	responseStr := string(response[:n])
	if !contains(responseStr, "200") && !contains(responseStr, "201") {
		conn.Close()
		return nil, fmt.Errorf("proxy rejected connection: %s", responseStr[:min(200, n)])
	}

	p.log(LevelDebug, fmt.Sprintf("Connected to %s through proxy %s", targetHost, proxyAddr))
	return conn, nil
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s[:len(substr)] == substr || len(s) > len(substr) && s[1:len(substr)+1] == substr || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleHTTP handles regular HTTP requests (non-CONNECT)
func (p *ProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	p.log(LevelDebug, fmt.Sprintf(">> 收到 HTTP 请求: %s %s", r.Method, r.URL.String()))

	// Create the outgoing request
	outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			outReq.Header.Add(key, value)
		}
	}

	// Remove hop-by-hop headers
	outReq.Header.Del("Proxy-Connection")
	outReq.Header.Del("Proxy-Authenticate")
	outReq.Header.Del("Proxy-Authorization")

	// Create HTTP client with optional proxy
	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	// Configure upstream proxy if specified
	if p.httpProxy != "" {
		proxyURL, err := url.Parse(p.httpProxy)
		if err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
			p.log(LevelDebug, fmt.Sprintf("Using upstream HTTP proxy: %s", p.httpProxy))
		}
	}

	resp, err := client.Do(outReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Send status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body (streaming)
	if flusher, ok := w.(http.Flusher); ok {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}
}
