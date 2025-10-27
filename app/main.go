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
		// Handle each connection concurrently:
		go handleConn(conn)
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

	// 2) Leer headers en un mapa (case-insensitive → claves en minúsculas)
	headers, ok := readHeaders(reader)
	if !ok {
		return
	}

	// 3) Routing mínimo
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
		ua := headers["user-agent"] // puede no existir, pero el tester sí lo envía
		body := []byte(ua)
		h := map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		}
		writeResponse(conn, "HTTP/1.1 200 OK", h, body)

	default:
		writeResponse(conn, "HTTP/1.1 404 Not Found", map[string]string{}, nil)
	}
}

// readHeaders lee líneas hasta el CRLF en blanco y devuelve un mapa nombre→valor.
// - Nombres en minúsculas (case-insensitive).
// - Recorta espacios alrededor del nombre y del valor.
// - Si hay headers repetidos, concatena con coma (comportamiento típico HTTP/1.1 simple).
func readHeaders(r *bufio.Reader) (map[string]string, bool) {
	h := make(map[string]string)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, false
		}
		if line == "\r\n" {
			break // fin de headers
		}
		line = strings.TrimRight(line, "\r\n")

		// nombre: valor
		name, value, found := strings.Cut(line, ":")
		if !found {
			// línea malformada; para este reto, lo ignoramos de forma indulgente
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