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
				"5\r\nhello\r\n" +
				"0\r\n\r\n",
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
			Name:        "TC-04: Newlines inside chunk data",
			Description: "Newline characters inside chunk data must be treated as body bytes",
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
			Description: "Chunk size is not valid hexadecimal",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"ZZ\r\nhello\r\n" +
				"0\r\n\r\n",
			Expected: "Parser error: invalid chunk size",
		},
		{
			Name:        "TC-07: Invalid chunk terminator",
			Description: "Chunk data must be followed by CRLF",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"5\r\nhelloXX" +
				"0\r\n\r\n",
			Expected: "Parser error: invalid chunk terminator",
		},
		{
			Name:        "TC-08: Incomplete chunk",
			Description: "Declared 10 bytes but only 5 are currently available",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"A\r\nhello",
			Expected: "Parser waits for more bytes",
		},

		// ==========================================
		// CASE-INSENSITIVE HEADER NAMES
		// ==========================================
		{
			Name:        "TC-09: Lowercase Content-Length",
			Description: "HTTP field names are case-insensitive",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"content-length: 5\r\n\r\n" +
				"hello",
			Expected: `Body: "hello", State: COMPLETE`,
		},
		{
			Name:        "TC-10: Uppercase Content-Length",
			Description: "Uppercase field name must behave like Content-Length",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"CONTENT-LENGTH: 5\r\n\r\n" +
				"hello",
			Expected: `Body: "hello", State: COMPLETE`,
		},
		{
			Name:        "TC-11: Mixed-case Transfer-Encoding",
			Description: "Transfer-Encoding field name is case-insensitive",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"tRaNsFeR-EnCoDiNg: chunked\r\n\r\n" +
				"5\r\nhello\r\n" +
				"0\r\n\r\n",
			Expected: `Body: "hello", State: COMPLETE`,
		},

		// ==========================================
		// CONTENT-LENGTH
		// ==========================================
		{
			Name:        "TC-12: Exact Content-Length",
			Description: "Exactly five body octets are supplied",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 5\r\n\r\n" +
				"hello",
			Expected: `Body: "hello", State: COMPLETE`,
		},
		{
			Name:        "TC-13: JSON body",
			Description: "Content-Length counts octets of the body",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Type: application/json\r\n" +
				"Content-Length: 16\r\n\r\n" +
				`{"name":"Nazar"}`,
			Expected: `Body: "{\"name\":\"Nazar\"}", State: COMPLETE`,
		},
		{
			Name:        "TC-14: Content-Length zero",
			Description: "Explicit zero-length body",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 0\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-15: Incomplete Content-Length body",
			Description: "Declared 10 bytes but only 5 have arrived",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 10\r\n\r\n" +
				"hello",
			Expected: "Parser waits for 5 more body bytes",
		},
		{
			Name:        "TC-16: Bytes after Content-Length body",
			Description: "Bytes after the declared body belong to subsequent input, not this body",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 5\r\n\r\n" +
				"hello" +
				"GET /next HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `First Body: "hello", State: COMPLETE; remaining bytes begin with "GET /next"`,
		},
		{
			Name:        "TC-17: Invalid Content-Length text",
			Description: "Content-Length must contain a valid decimal length",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: abc\r\n\r\n" +
				"hello",
			Expected: "Parser error: invalid Content-Length",
		},
		{
			Name:        "TC-18: Negative Content-Length",
			Description: "Negative Content-Length is invalid",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: -5\r\n\r\n" +
				"hello",
			Expected: "Parser error: invalid Content-Length",
		},
		{
			Name:        "TC-19: Empty Content-Length",
			Description: "Empty Content-Length value is invalid",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length:\r\n\r\n",
			Expected: "Parser error: invalid Content-Length",
		},

		// ==========================================
		// REQUEST WITHOUT BODY FRAMING
		// ==========================================
		{
			Name:        "TC-20: GET without body",
			Description: "No Content-Length and no Transfer-Encoding means request body length is zero",
			Request: "GET /users HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-21: DELETE without body",
			Description: "Body framing does not depend on method name",
			Request: "DELETE /users/1 HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-22: POST without body",
			Description: "POST also has zero body when no framing header is present",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `Body: "", State: COMPLETE`,
		},
		{
			Name:        "TC-23: Data after unframed request",
			Description: "Without body framing, bytes after headers must not become this request's body",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n" +
				"GET /next HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n\r\n",
			Expected: `First Body: "", State: COMPLETE; remaining bytes begin with "GET /next"`,
		},

		// ==========================================
		// CONTENT-LENGTH + TRANSFER-ENCODING CONFLICT
		// ==========================================
		{
			Name:        "TC-24: Content-Length and Transfer-Encoding",
			Description: "Request contains both framing mechanisms and must be rejected by this implementation",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Content-Length: 5\r\n" +
				"Transfer-Encoding: chunked\r\n\r\n" +
				"5\r\nhello\r\n" +
				"0\r\n\r\n",
			Expected: "Parser error: Content-Length and Transfer-Encoding conflict",
		},
		{
			Name:        "TC-25: Transfer-Encoding before Content-Length",
			Description: "Detection must not depend on header order",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"Transfer-Encoding: chunked\r\n" +
				"Content-Length: 5\r\n\r\n" +
				"5\r\nhello\r\n" +
				"0\r\n\r\n",
			Expected: "Parser error: Content-Length and Transfer-Encoding conflict",
		},
		{
			Name:        "TC-26: Conflict with mixed header casing",
			Description: "Conflict detection must also use case-insensitive field names",
			Request: "POST /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"content-length: 5\r\n" +
				"TRANSFER-ENCODING: chunked\r\n\r\n" +
				"5\r\nhello\r\n" +
				"0\r\n\r\n",
			Expected: "Parser error: Content-Length and Transfer-Encoding conflict",
		},
		{
			Name:        "TC-27: No body",
			Description: "Request doesn't include body",
			Request: "GET /echo HTTP/1.1\r\n" +
				"Host: localhost:4000\r\n" +
				"\r\n",
			Expected: "OK",
		},
	}

	// for i := 0; i < 1; i++ {
	for i := 0; i < len(tests); i++ {
		runTest(tests[i])
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
