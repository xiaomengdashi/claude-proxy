package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// ProxyServer handles HTTP CONNECT proxy requests
type ProxyServer struct {
	port     int
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
}

// NewProxyServer creates a new proxy server instance
func NewProxyServer(port int) *ProxyServer {
	return &ProxyServer{
		port: port,
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

	fmt.Printf("[Proxy] Listening on %s\n", addr)
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
	fmt.Printf("[Proxy] CONNECT %s\n", r.Host)

	// Connect to the target server
	targetConn, err := net.DialTimeout("tcp", r.Host, 30*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to %s: %v", r.Host, err), http.StatusBadGateway)
		fmt.Printf("[Proxy] Failed to connect to %s: %v\n", r.Host, err)
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
		fmt.Printf("[Proxy] Failed to send connection established: %v\n", err)
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
	fmt.Printf("[Proxy] CONNECT %s completed\n", r.Host)
}

// handleHTTP handles regular HTTP requests (non-CONNECT)
func (p *ProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("[Proxy] %s %s\n", r.Method, r.URL.String())

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

	// Send request
	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
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
