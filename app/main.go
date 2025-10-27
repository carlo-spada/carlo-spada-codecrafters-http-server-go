package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var baseDir string // set by --directory flag (absolute path)

func main() {
	fmt.Println("Logs from your program will appear here!")

	// Parse --directory flag (simple hand-rolled parser)
	for i := 0; i < len(os.Args); i++ {
		if os.Args[i] == "--directory" && i+1 < len(os.Args) {
			baseDir = os.Args[i+1]
			if abs, err := filepath.Abs(baseDir); err == nil {
				baseDir = abs
			}
			i++
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
	_ = version // not used yet

	// 2) Headers → map (lower-case keys)
	headers, ok := readHeaders(reader)
	if !ok {
		return
	}

	switch {
	// ----- GETs we already support -----
	case method == "GET" && path == "/":
		writeResponse(conn, "HTTP/1.1 200 OK", map[string]string{}, nil)

	case method == "GET" && strings.HasPrefix(path, "/echo/"):
		msg := strings.TrimPrefix(path, "/echo/")
		body := []byte(msg)
		writeResponse(conn, "HTTP/1.1 200 OK", map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		}, body)

	case method == "GET" && path == "/user-agent":
		ua := headers["user-agent"]
		body := []byte(ua)
		writeResponse(conn, "HTTP/1.1 200 OK", map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		}, body)

	case strings.HasPrefix(path, "/files/") && method == "GET":
		// Serve existing file
		if baseDir == "" {
			writeResponse(conn, "HTTP/1.1 404 Not Found", nil, nil)
			return
		}
		raw := strings.TrimPrefix(path, "/files/")
		filename, err := url.PathUnescape(raw)
		if err != nil {
			writeResponse(conn, "HTTP/1.1 404 Not Found", nil, nil)
			return
		}
		full, ok := safeJoin(baseDir, filename)
		if !ok {
			writeResponse(conn, "HTTP/1.1 404 Not Found", nil, nil)
			return
		}
		data, err := os.ReadFile(full)
		if err != nil {
			writeResponse(conn, "HTTP/1.1 404 Not Found", nil, nil)
			return
		}
		writeResponse(conn, "HTTP/1.1 200 OK", map[string]string{
			"Content-Type":   "application/octet-stream",
			"Content-Length": fmt.Sprintf("%d", len(data)),
		}, data)

	// ----- NEW: POST /files/{filename} -----
	case strings.HasPrefix(path, "/echo/"):
	msg := strings.TrimPrefix(path, "/echo/")
	body := []byte(msg)

	h := map[string]string{
		"Content-Type":   "text/plain",
		"Content-Length": fmt.Sprintf("%d", len(body)),
	}
	// NEW: negotiate gzip (header only; body stays plain for this stage)
	if acceptsGzip(headers) {
		h["Content-Encoding"] = "gzip"
	}

	writeResponse(conn, "HTTP/1.1 200 OK", h, body)

		// Decode filename
		raw := strings.TrimPrefix(path, "/files/")
		filename, err := url.PathUnescape(raw)
		if err != nil {
			writeResponse(conn, "HTTP/1.1 404 Not Found", nil, nil)
			return
		}
		full, ok := safeJoin(baseDir, filename)
		if !ok {
			writeResponse(conn, "HTTP/1.1 404 Not Found", nil, nil)
			return
		}

		// Read body exactly Content-Length bytes
		clStr := headers["content-length"]
		if clStr == "" {
			// For this stage the tester always sends Content-Length, but guard anyway
			writeResponse(conn, "HTTP/1.1 411 Length Required", nil, nil)
			return
		}
		cl, err := strconv.Atoi(clStr)
		if err != nil || cl < 0 {
			writeResponse(conn, "HTTP/1.1 400 Bad Request", nil, nil)
			return
		}
			data := make([]byte, cl)
			if _, err = io.ReadFull(reader, data); err != nil {
				// not enough bytes
				writeResponse(conn, "HTTP/1.1 400 Bad Request", nil, nil)
				return
			}

			// Write file (0644)
			if err := os.WriteFile(full, data, 0o644); err != nil {
				// If directory missing or permission error, simplest is 404/500.
				// The spec for the stage only checks success path; 500 is reasonable here.
				writeResponse(conn, "HTTP/1.1 500 Internal Server Error", nil, nil)
				return
			}

			// Success: 201 Created, no body required
			writeResponse(conn, "HTTP/1.1 201 Created", map[string]string{}, nil)

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

// safeJoin ensures target stays within base.
func safeJoin(base, name string) (string, bool) {
	if filepath.IsAbs(name) {
		return "", false
	}
	target := filepath.Join(base, name)
	absBase, err1 := filepath.Abs(base)
	absTarget, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return "", false
	}
	absBase = filepath.Clean(absBase)
	absTarget = filepath.Clean(absTarget)
	baseWithSep := absBase + string(os.PathSeparator)
	if absTarget == absBase || strings.HasPrefix(absTarget, baseWithSep) {
		return absTarget, true
	}
	return "", false
}

func acceptsGzip(headers map[string]string) bool {
	ae := headers["accept-encoding"]
	if ae == "" {
		return false
	}
	// Tokenize on comma, strip spaces, compare case-insensitively.
	for _, tok := range strings.Split(ae, ",") {
		if strings.EqualFold(strings.TrimSpace(tok), "gzip") {
			return true
		}
	}
	return false
}
