package main

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var baseDir string // set by --directory flag (absolute path)

func main() {
	fmt.Println("Logs from your program will appear here!")

	// Parse --directory flag (simple hand-rolled parser)
	for i := 0; i < len(os.Args); i++ {
		if os.Args[i] == "--directory" && i+1 < len(os.Args) {
			baseDir = os.Args[i+1]
			abs, err := filepath.Abs(baseDir)
			if err == nil {
				baseDir = abs
			}
			i++ // skip value
		}
	}

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		// concurrency stage: each connection in its own goroutine
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// 1) Request line
	reqLine, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	reqLine = strings.TrimRight(reqLine, "\r\n")

	parts := strings.SplitN(reqLine, " ", 3)
	if len(parts) < 3 {
		return
	}
	method, path, version := parts[0], parts[1], parts[2]
	_ = method
	_ = version

	// 2) Headers → map (case-insensitive)
	headers, ok := readHeaders(reader)
	if !ok {
		return
	}

	// 3) Routing
	switch {
	case path == "/":
		writeResponse(conn, "HTTP/1.1 200 OK", map[string]string{}, nil)

	case strings.HasPrefix(path, "/echo/"):
		msg := strings.TrimPrefix(path, "/echo/")
		body := []byte(msg)
		h := map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		}
		writeResponse(conn, "HTTP/1.1 200 OK", h, body)

	case path == "/user-agent":
		ua := headers["user-agent"]
		body := []byte(ua)
		h := map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		}
		writeResponse(conn, "HTTP/1.1 200 OK", h, body)

	case strings.HasPrefix(path, "/files/"):
		// /files/{filename}
		raw := strings.TrimPrefix(path, "/files/")
		// URL-decode: supports %20 etc.
		filename, err := url.PathUnescape(raw)
		if err != nil {
			// Bad path decode → 404 is fine for this stage
			writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
			return
		}

		// Safe-join against baseDir (if provided)
		if baseDir == "" {
			// The tester always passes --directory, but be defensive
			writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
			return
		}
		full, ok := safeJoin(baseDir, filename)
		if !ok {
			// attempted path traversal or invalid join
			writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
			return
		}

		// Read file
		data, err := os.ReadFile(full)
		if err != nil {
			// not found or unreadable → 404 for this stage
			writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
			return
		}

		h := map[string]string{
			"Content-Type":   "application/octet-stream",
			"Content-Length": fmt.Sprintf("%d", len(data)),
		}
		writeResponse(conn, "HTTP/1.1 200 OK", h, data)

	default:
		writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
	}
}

func readHeaders(r *bufio.Reader) (map[string]string, bool) {
	h := make(map[string]string)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, false
		}
		if line == "\r\n" {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)

		if prev, exists := h[name]; exists && prev != "" && value != "" {
			h[name] = prev + ", " + value
		} else {
			h[name] = value
		}
	}
	return h, true
}

func writeResponse(conn net.Conn, statusLine string, headers map[string]string, body []byte) {
	fmt.Fprintf(conn, "%s\r\n", statusLine)
	for k, v := range headers {
		fmt.Fprintf(conn, "%s: %s\r\n", k, v)
	}
	fmt.Fprint(conn, "\r\n")
	if len(body) > 0 {
		conn.Write(body)
	}
}

// safeJoin joins base + name and ensures the result stays within baseDir.
// Returns (cleanPath, true) if safe; otherwise ("", false).
func safeJoin(base, name string) (string, bool) {
	// prevent absolute names from skipping base
	if filepath.IsAbs(name) {
		return "", false
	}
	target := filepath.Join(base, name)
	absBase, err1 := filepath.Abs(base)
	absTarget, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return "", false
	}
	// ensure absTarget is inside absBase (prefix match on path components)
	absBase = filepath.Clean(absBase)
	absTarget = filepath.Clean(absTarget)
	// Add trailing separator to avoid false positives (e.g., /tmp/a vs /tmp/ab)
	baseWithSep := absBase + string(os.PathSeparator)
	if absTarget == absBase || strings.HasPrefix(absTarget+string(os.PathSeparator), baseWithSep) || strings.HasPrefix(absTarget, baseWithSep) {
		// The equality case means requesting the directory itself; usually not desired.
		// But we allow it to pass; file read will likely fail and yield 404.
		return absTarget, true
	}
	return "", false
}