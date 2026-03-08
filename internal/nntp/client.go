package nntp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client represents a single NNTP connection.
type Client struct {
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
	server   string
	mu       sync.Mutex
	lastUsed time.Time
}

// ServerConfig holds connection parameters for a Usenet server.
type ServerConfig struct {
	Host     string
	Port     int
	TLS      bool
	Username string
	Password string
}

// Dial establishes a new NNTP connection.
func Dial(ctx context.Context, cfg ServerConfig) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	dialer := &net.Dialer{Timeout: 30 * time.Second}

	var conn net.Conn
	var err error

	if cfg.TLS {
		tlsCfg := &tls.Config{
			ServerName: cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}

	client := &Client{
		conn:     conn,
		reader:   bufio.NewReaderSize(conn, 64*1024), // 64KB read buffer
		writer:   bufio.NewWriter(conn),
		server:   addr,
		lastUsed: time.Now(),
	}

	// Read server greeting
	code, _, err := client.readResponse()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("reading greeting: %w", err)
	}
	if code != 200 && code != 201 {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected greeting code: %d", code)
	}

	// Authenticate if credentials provided
	if cfg.Username != "" {
		if err := client.authenticate(cfg.Username, cfg.Password); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	slog.Debug("nntp connected", "server", addr, "tls", cfg.TLS)
	return client, nil
}

// authenticate sends AUTHINFO USER/PASS commands.
func (c *Client) authenticate(user, pass string) error {
	code, _, err := c.sendCommand("AUTHINFO USER " + user)
	if err != nil {
		return err
	}

	if code == 281 {
		return nil // Auth accepted with just username
	}
	if code != 381 {
		return fmt.Errorf("unexpected response to USER: %d", code)
	}

	code, msg, err := c.sendCommand("AUTHINFO PASS " + pass)
	if err != nil {
		return err
	}
	if code != 281 {
		return fmt.Errorf("auth rejected: %d %s", code, msg)
	}

	return nil
}

// Body fetches the body of an article by Message-ID.
// Returns a reader for the article body (yEnc encoded data).
func (c *Client) Body(messageID string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastUsed = time.Now()

	// Message-IDs are wrapped in angle brackets
	id := messageID
	if !strings.HasPrefix(id, "<") {
		id = "<" + id + ">"
	}

	code, msg, err := c.sendCommand("BODY " + id)
	if err != nil {
		return nil, err
	}

	switch code {
	case 222:
		// Success - read multi-line body
		return c.readMultiLine()
	case 430:
		return nil, fmt.Errorf("article not found: %s", messageID)
	case 423:
		return nil, fmt.Errorf("no article with that number: %s", messageID)
	default:
		return nil, fmt.Errorf("unexpected BODY response: %d %s", code, msg)
	}
}

// sendCommand writes a command and reads the single-line response.
func (c *Client) sendCommand(cmd string) (int, string, error) {
	_, err := c.writer.WriteString(cmd + "\r\n")
	if err != nil {
		return 0, "", fmt.Errorf("writing command: %w", err)
	}
	if err := c.writer.Flush(); err != nil {
		return 0, "", fmt.Errorf("flushing command: %w", err)
	}
	return c.readResponse()
}

// readResponse reads a single-line NNTP response: "CODE message"
func (c *Client) readResponse() (int, string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("reading response: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")

	if len(line) < 3 {
		return 0, "", fmt.Errorf("response too short: %q", line)
	}

	code, err := strconv.Atoi(line[:3])
	if err != nil {
		return 0, "", fmt.Errorf("parsing response code: %w", err)
	}

	msg := ""
	if len(line) > 4 {
		msg = line[4:]
	}

	return code, msg, nil
}

// readMultiLine reads a multi-line NNTP response (terminated by ".\r\n").
// Returns the raw bytes for the decoder to process.
func (c *Client) readMultiLine() ([]byte, error) {
	var buf []byte
	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("reading body: %w", err)
		}

		// Dot-stuffing: lines starting with ".." have the first dot removed
		// A line that is just ".\r\n" terminates the response
		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "." {
			break
		}
		if strings.HasPrefix(trimmed, "..") {
			line = line[1:] // Remove dot-stuffing
		}

		buf = append(buf, line...)
	}
	return buf, nil
}

// Close terminates the NNTP connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _, _ = c.sendCommand("QUIT")
	return c.conn.Close()
}

// Alive checks if the connection is still responsive.
func (c *Client) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = c.conn.SetDeadline(time.Time{}) }()

	code, _, err := c.sendCommand("DATE")
	return err == nil && code == 111
}

// --- Connection Pool ---

// Pool manages a pool of NNTP connections to a single server.
type Pool struct {
	cfg      ServerConfig
	maxConns int
	conns    chan *Client
	mu       sync.Mutex
	active   int
}

// NewPool creates a connection pool for the given server.
func NewPool(cfg ServerConfig, maxConns int) *Pool {
	return &Pool{
		cfg:      cfg,
		maxConns: maxConns,
		conns:    make(chan *Client, maxConns),
	}
}

// MaxConns returns the maximum number of connections for this pool.
func (p *Pool) MaxConns() int {
	return p.maxConns
}

// Get retrieves a connection from the pool, creating one if needed.
func (p *Pool) Get(ctx context.Context) (*Client, error) {
	// Try to get an existing connection
	select {
	case client := <-p.conns:
		if client.Alive() {
			return client, nil
		}
		_ = client.Close()
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	default:
	}

	// Create new connection if under limit
	p.mu.Lock()
	if p.active >= p.maxConns {
		p.mu.Unlock()
		// Wait for one to be returned
		select {
		case client := <-p.conns:
			return client, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.active++
	p.mu.Unlock()

	client, err := Dial(ctx, p.cfg)
	if err != nil {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
		return nil, err
	}

	return client, nil
}

// Put returns a connection to the pool.
func (p *Pool) Put(client *Client) {
	select {
	case p.conns <- client:
	default:
		// Pool is full, close the connection
		_ = client.Close()
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}
}

// Close drains and closes all connections in the pool.
func (p *Pool) Close() {
	close(p.conns)
	for client := range p.conns {
		_ = client.Close()
	}
}
