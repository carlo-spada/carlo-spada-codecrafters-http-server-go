package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	fmt.Println("Logs from your program will appear here!")

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
		handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// 1) Request line: "GET /algo HTTP/1.1\r\n"
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

	// 2) Consumir headers hasta línea en blanco
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" {
			break
		}
	}

	// 3) Routing mínimo
	switch {
	case path == "/":
		// 200 sin cuerpo (como etapa anterior)
		writeResponse(conn, "HTTP/1.1 200 OK", map[string]string{}, nil)

	case strings.HasPrefix(path, "/echo/"):
		msg := strings.TrimPrefix(path, "/echo/")
		body := []byte(msg)
		headers := map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		}
		writeResponse(conn, "HTTP/1.1 200 OK", headers, body)

	default:
		// 404 sin cuerpo
		writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
	}
}

func writeResponse(conn net.Conn, statusLine string, headers map[string]string, body []byte) {
	// Status line
	fmt.Fprintf(conn, "%s\r\n", statusLine)
	// Headers
	for k, v := range headers {
		fmt.Fprintf(conn, "%s: %s\r\n", k, v)
	}
	// Blank line
	fmt.Fprint(conn, "\r\n")
	// Body
	if len(body) > 0 {
		conn.Write(body)
	}
}