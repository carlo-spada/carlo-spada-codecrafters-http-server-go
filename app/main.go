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

	// Bucle para aceptar varias conexiones (el tester hará + de 1 request)
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		// Manejo en una función separada (limpio y extensible)
		handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// 1) Leer la primera línea del request (request line)
	// Ej: "GET /algo HTTP/1.1\r\n"
	reqLine, err := reader.ReadString('\n')
	if err != nil {
		// Si no pudimos leer una línea completa, cerramos silenciosamente.
		return
	}

	// Quitar terminadores de línea CRLF
	reqLine = strings.TrimRight(reqLine, "\r\n")

	// 2) Partir por espacios: método, path, versión
	// Esperamos 3 fragmentos
	parts := strings.SplitN(reqLine, " ", 3)
	if len(parts) < 3 {
		// Request malformado, respondemos 400 o simplemente cerramos (para este stage, basta cerrar)
		return
	}
	method, path, version := parts[0], parts[1], parts[2]
	_ = method  // por ahora no lo usamos
	_ = version // por ahora no lo usamos

	// 3) (Opcional en este stage) Consumir/descartar los headers hasta la línea en blanco
	// El reto dice que podemos ignorarlos, pero leer hasta CRLF en blanco evita que
	// queden bytes “colgados” si el cliente reusa la conexión.
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" {
			break // fin de headers
		}
	}

	// 4) Decidir la respuesta según el path
	var response string
	if path == "/" {
		response = "HTTP/1.1 200 OK\r\n\r\n"
	} else {
		response = "HTTP/1.1 404 Not Found\r\n\r\n"
	}

	// 5) Escribir la respuesta
	if _, err := conn.Write([]byte(response)); err != nil {
		fmt.Println("write failed:", err)
	}
}