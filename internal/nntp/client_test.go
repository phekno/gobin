package nntp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestReadResponse_Valid(t *testing.T) {
	c := &Client{
		reader: bufio.NewReader(strings.NewReader("200 Welcome\r\n")),
	}
	code, msg, err := c.readResponse()
	if err != nil {
		t.Fatalf("readResponse failed: %v", err)
	}
	if code != 200 {
		t.Errorf("code = %d, want 200", code)
	}
	if msg != "Welcome" {
		t.Errorf("msg = %q, want Welcome", msg)
	}
}

func TestReadResponse_CodeOnly(t *testing.T) {
	c := &Client{
		reader: bufio.NewReader(strings.NewReader("200\r\n")),
	}
	code, msg, err := c.readResponse()
	if err != nil {
		t.Fatalf("readResponse failed: %v", err)
	}
	if code != 200 {
		t.Errorf("code = %d, want 200", code)
	}
	if msg != "" {
		t.Errorf("msg = %q, want empty", msg)
	}
}

func TestReadResponse_ShortLine(t *testing.T) {
	c := &Client{
		reader: bufio.NewReader(strings.NewReader("OK\r\n")),
	}
	_, _, err := c.readResponse()
	if err == nil {
		t.Error("expected error for short response")
	}
}

func TestReadResponse_NonNumericCode(t *testing.T) {
	c := &Client{
		reader: bufio.NewReader(strings.NewReader("ABC message\r\n")),
	}
	_, _, err := c.readResponse()
	if err == nil {
		t.Error("expected error for non-numeric code")
	}
}

func TestReadMultiLine(t *testing.T) {
	input := "line one\r\nline two\r\n.\r\n"
	c := &Client{
		reader: bufio.NewReader(strings.NewReader(input)),
	}
	data, err := c.readMultiLine()
	if err != nil {
		t.Fatalf("readMultiLine failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "line one") {
		t.Error("missing 'line one'")
	}
	if !strings.Contains(s, "line two") {
		t.Error("missing 'line two'")
	}
	if strings.HasSuffix(strings.TrimSpace(s), ".") {
		t.Error("terminating dot should not be in output")
	}
}

func TestReadMultiLine_DotStuffing(t *testing.T) {
	input := "..This starts with a dot\r\n.\r\n"
	c := &Client{
		reader: bufio.NewReader(strings.NewReader(input)),
	}
	data, err := c.readMultiLine()
	if err != nil {
		t.Fatalf("readMultiLine failed: %v", err)
	}
	if !strings.Contains(string(data), ".This starts with a dot") {
		t.Errorf("dot-stuffing not removed, got %q", string(data))
	}
}

// fakeNNTPServer starts a local TCP listener that speaks minimal NNTP.
// The handler function is called with the accepted connection.
// Returns host and port for the test client to connect to.
func fakeNNTPServer(t *testing.T, handler func(net.Conn)) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handler(conn)
		_ = conn.Close()
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestDial_BasicConnection(t *testing.T) {
	host, port := fakeNNTPServer(t, func(conn net.Conn) {
		_, _ = fmt.Fprintf(conn, "200 Welcome\r\n")
		// Read and discard QUIT
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
		_, _ = fmt.Fprintf(conn, "205 Bye\r\n")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Dial(ctx, ServerConfig{Host: host, Port: port, TLS: false})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	if client.server != fmt.Sprintf("%s:%d", host, port) {
		t.Errorf("server = %q", client.server)
	}
}

func TestDial_BadGreeting(t *testing.T) {
	host, port := fakeNNTPServer(t, func(conn net.Conn) {
		_, _ = fmt.Fprintf(conn, "502 Access denied\r\n")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Dial(ctx, ServerConfig{Host: host, Port: port, TLS: false})
	if err == nil {
		t.Error("expected error for 502 greeting")
	}
}

func TestDial_WithAuth(t *testing.T) {
	host, port := fakeNNTPServer(t, func(conn net.Conn) {
		_, _ = fmt.Fprintf(conn, "200 Welcome\r\n")
		scanner := bufio.NewScanner(conn)

		// Expect AUTHINFO USER
		if scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "AUTHINFO USER") {
				_, _ = fmt.Fprintf(conn, "500 unexpected\r\n")
				return
			}
			_, _ = fmt.Fprintf(conn, "381 Password required\r\n")
		}

		// Expect AUTHINFO PASS
		if scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "AUTHINFO PASS") {
				_, _ = fmt.Fprintf(conn, "500 unexpected\r\n")
				return
			}
			_, _ = fmt.Fprintf(conn, "281 Authentication accepted\r\n")
		}

		// Handle QUIT
		if scanner.Scan() {
			_, _ = fmt.Fprintf(conn, "205 Bye\r\n")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Dial(ctx, ServerConfig{
		Host:     host,
		Port:     port,
		TLS:      false,
		Username: "testuser",
		Password: "testpass",
	})
	if err != nil {
		t.Fatalf("Dial with auth failed: %v", err)
	}
	client.Close()
}

func TestBody_Success(t *testing.T) {
	host, port := fakeNNTPServer(t, func(conn net.Conn) {
		_, _ = fmt.Fprintf(conn, "200 Welcome\r\n")
		scanner := bufio.NewScanner(conn)

		if scanner.Scan() {
			// Expect BODY <message-id>
			line := scanner.Text()
			if !strings.Contains(line, "<test@msg>") {
				_, _ = fmt.Fprintf(conn, "430 No such article\r\n")
				return
			}
			_, _ = fmt.Fprintf(conn, "222 body follows\r\n")
			_, _ = fmt.Fprintf(conn, "this is the body\r\n")
			_, _ = fmt.Fprintf(conn, ".\r\n")
		}

		// Handle QUIT
		if scanner.Scan() {
			_, _ = fmt.Fprintf(conn, "205 Bye\r\n")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Dial(ctx, ServerConfig{Host: host, Port: port})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	data, err := client.Body("test@msg")
	if err != nil {
		t.Fatalf("Body failed: %v", err)
	}
	if !strings.Contains(string(data), "this is the body") {
		t.Errorf("Body data = %q", string(data))
	}
}

func TestBody_NotFound(t *testing.T) {
	host, port := fakeNNTPServer(t, func(conn net.Conn) {
		_, _ = fmt.Fprintf(conn, "200 Welcome\r\n")
		scanner := bufio.NewScanner(conn)

		if scanner.Scan() {
			_, _ = fmt.Fprintf(conn, "430 No such article\r\n")
		}

		if scanner.Scan() {
			_, _ = fmt.Fprintf(conn, "205 Bye\r\n")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Dial(ctx, ServerConfig{Host: host, Port: port})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	_, err = client.Body("missing@msg")
	if err == nil {
		t.Error("expected error for missing article")
	}
}

func TestPool_GetAndPut(t *testing.T) {
	host, port := fakeNNTPServer(t, func(conn net.Conn) {
		_, _ = fmt.Fprintf(conn, "200 Welcome\r\n")
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "DATE") {
				_, _ = fmt.Fprintf(conn, "111 20260308120000\r\n")
			} else if strings.HasPrefix(line, "QUIT") {
				_, _ = fmt.Fprintf(conn, "205 Bye\r\n")
				return
			}
		}
	})

	pool := NewPool(ServerConfig{Host: host, Port: port}, 2)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Pool.Get failed: %v", err)
	}

	pool.Put(client)

	// Get again — should reuse from pool
	client2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Pool.Get (reuse) failed: %v", err)
	}
	pool.Put(client2)
}
