package main

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type TestCase struct {
	Name        string
	Description string
	Request     string
	Expected    string
}

func main() {
	tests := []TestCase{
		// ==========================================
		// TRANSFER-ENCODING: CHUNKED
		// ==========================================
		{
			Name:        "TC-01: Single chunk",
			Description: "One chunk containing hello",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"5\r\nhello\r\n0\r\n\r\n",
			Expected: `Body: "hello", State: COMPLETE`,
		},
		{
			Name:        "TC-02: Multiple chunks",
			Description: "Two chunks forming hello world",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"6\r\nhello \r\n" +
				"5\r\nworld\r\n" +
				"0\r\n\r\n",
			Expected: `Body: "hello world", State: COMPLETE`,
		},
		{
			Name:        "TC-03: Empty chunked body",
			Description: "Zero-sized terminating chunk",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"0\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-04: Newlines inside body",
			Description: "Newline characters are body data",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"5\r\nhello\r\n" +
				"2\r\n\n\n\r\n" +
				"5\r\nworld\r\n" +
				"0\r\n\r\n",
			Expected: `Body: "hello\n\nworld", State: COMPLETE`,
		},
		{
			Name:        "TC-05: Hexadecimal chunk size",
			Description: "Chunk size A represents 10 bytes",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"A\r\n0123456789\r\n" +
				"0\r\n\r\n",
			Expected: `Body: "0123456789", State: COMPLETE`,
		},
		{
			Name:        "TC-06: Invalid chunk size",
			Description: "Invalid hexadecimal size",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"ZZ\r\nhello\r\n" +
				"0\r\n\r\n",
			Expected: "Parser error: invalid chunk size",
		},
		{
			Name:        "TC-07: Invalid chunk terminator",
			Description: "Chunk data is not followed by CRLF",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"5\r\nhelloXX" +
				"0\r\n\r\n",
			Expected: "Parser error: invalid chunk terminator",
		},
		{
			Name:        "TC-08: Incomplete chunk",
			Description: "Declared 10 bytes but only 5 supplied",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"A\r\nhello",
			Expected: "Parser waits for more bytes",
		},

		// ==========================================
		// CONTENT-LENGTH
		// ==========================================
		{
			Name:        "TC-09: Simple Content-Length",
			Description: "Body of exactly 5 bytes",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 5\r\n\r\n" +
				"hello",
			Expected: `Body: "hello", State: COMPLETE`,
		},
		{
			Name:        "TC-10: JSON body",
			Description: "Body containing JSON",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Type: application/json\r\n" +
				"Content-Length: 16\r\n\r\n" +
				`{"name":"Nazar"}`,
			Expected: `Body: {"name":"Nazar"}, State: COMPLETE`,
		},
		{
			Name:        "TC-11: Content-Length zero",
			Description: "Explicitly empty body",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 0\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-12: Body shorter than Content-Length",
			Description: "Declared 10 bytes, sent only 5",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 10\r\n\r\n" +
				"hello",
			Expected: "Parser waits for more bytes",
		},
		{
			Name:        "TC-13: Body longer than Content-Length",
			Description: "Declared 3 bytes, sent 5",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 3\r\n\r\n" +
				"hello",
			Expected: "First body contains hel; extra lo remains unread",
		},
		{
			Name:        "TC-14: Invalid Content-Length",
			Description: "Non-numeric Content-Length",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: abc\r\n\r\n" +
				"hello",
			Expected: "Parser error: invalid Content-Length",
		},

		// ==========================================
		// NO CONTENT
		// ==========================================
		{
			Name:        "TC-15: GET without body",
			Description: "No Content-Length or Transfer-Encoding",
			Request: "GET /users HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-16: DELETE without body",
			Description: "DELETE request without body",
			Request: "DELETE /users/1 HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-17: POST without framing headers",
			Description: "POST without body framing",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-18: Body without framing headers",
			Description: "Unexpected bytes after a complete request",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n" +
				"hello",
			Expected: "Body remains empty; extra bytes cannot be used as this request's body",
		},
	}

	for _, tc := range tests {
		runTest(tc)
	}
}

func runTest(tc TestCase) {
	fmt.Println(strings.Repeat("=", 65))
	fmt.Println(tc.Name)
	fmt.Println("Description:", tc.Description)
	fmt.Println("Expected:", tc.Expected)

	conn, err := net.DialTimeout(
		"tcp",
		"127.0.0.1:4000",
		3*time.Second,
	)
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))

	_, err = io.WriteString(conn, tc.Request)
	if err != nil {
		fmt.Println("Write error:", err)
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	buffer := make([]byte, 1024)

	n, err := conn.Read(buffer)

	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("Actual: TIMEOUT (server did not respond)")
		} else if err == io.EOF {
			fmt.Println("Actual: Connection closed without response")
		} else {
			fmt.Println("Read error:", err)
		}
		return
	}

	fmt.Printf("Actual response: %q\n", string(buffer[:n]))
}
