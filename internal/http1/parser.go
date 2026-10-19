package http1

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type parserState int

const (
	stateMethod parserState = iota
	statePath
	stateProtocol
	stateHeaders
	stateBody
	stateComplete
)

type bodyParsingMethod int

const (
	unknown bodyParsingMethod = iota
	noContent
	contentLength
	transferEncoding
)

type parser struct {
	buffer            []byte
	request           Request
	state             parserState
	scanPos           int
	fieldStart        int
	bodyParsingMethod bodyParsingMethod
}

// MAIN PARSER ORCHESTRATOR
func (p *parser) parseHttpRequest(chunk []byte) error {
	p.buffer = append(p.buffer, chunk...)
	chunkLength := len(chunk)

	if p.state == stateBody && (p.bodyParsingMethod == unknown || p.bodyParsingMethod == noContent) && chunkLength == 0 {
		fmt.Printf("Headers include 'Content-Length' or 'Transfer-Encoding: chunked', but body is empty. Selected body framing method: %s", p.bodyParsingMethod)
		return errors.New("Invalid body.")
	}

	for chunkLength > 0 {
		switch p.state {
		case stateMethod, statePath, stateProtocol:
			bytesLeft, parseRequestLineError := p.parseRequestLine(chunkLength)
			if parseRequestLineError != nil {
				return parseRequestLineError
			}

			chunkLength = bytesLeft
		case stateHeaders:
			bytesLeft, headersErr := p.parseHeaders(chunkLength)
			if headersErr != nil {
				return headersErr
			}

			if p.state == stateBody && bytesLeft == 0 {
				chunkLength = 1
			} else {
				chunkLength = bytesLeft
			}
		case stateBody:
			// If request headers at this moment don't include 'Content-Length' or 'Transfer-Encoding: chunked' headers
			// then skip 'stateBody' and increment to the next state.
			if p.bodyParsingMethod == unknown {
				parsingMethod, getParsingMethodError := p.bodyParsingMethod.getBodyParsingMethod(p.request.Headers)
				if getParsingMethodError != nil {
					return getParsingMethodError
				}

				p.bodyParsingMethod = parsingMethod
			}

			if p.bodyParsingMethod == noContent {
				// Handling for cases where none of the suitable headers were passed,
				// but a body was provided.
				if chunkLength > 0 {
					return errors.New("Request contains body but headers don't include 'Content-Length' or 'Transfer-Encoding: chunked'.")
				}

				p.state++
				continue
			}
			if p.bodyParsingMethod == contentLength {
				bodyError := p.parseBodyByContentLength(chunkLength)
				if bodyError != nil {
					return bodyError
				}

				chunkLength = 0
			}
			if p.bodyParsingMethod == transferEncoding {
				bytesLeft, bodyError := p.parseBodyByChunks(chunkLength)
				if bodyError != nil {
					return bodyError
				}

				chunkLength = bytesLeft
			}
		case stateComplete:
			return nil
		}
	}
	return nil
}

// Method, Path, Protocol
func (p *parser) parseRequestLine(chunkLength int) (bytesLeft int, parseRequestLineError error) {
	for i := 0; i < chunkLength; i++ {
		if string(p.buffer[p.scanPos]) == "\n" {
			lineFinished := p.checkIfLineFinished()
			if lineFinished == true {
				p.request.Protocol = string(p.buffer[p.fieldStart : p.scanPos-1])
				p.state++
				p.fieldStart = p.scanPos + 1
				p.scanPos++
				return chunkLength - i - 1, nil
			}
		}

		if string(p.buffer[p.scanPos]) == " " {
			switch p.state {
			case stateMethod:
				p.request.Method = string(p.buffer[p.fieldStart:p.scanPos])
				p.state++
			case statePath:
				p.request.Path = string(p.buffer[p.fieldStart:p.scanPos])
				p.state++
			}

			p.fieldStart = p.scanPos + 1
		}

		p.scanPos++
	}
	return 0, nil
}

// Headers
func (p *parser) parseHeaders(chunkLength int) (bytesLeft int, parseHeadersError error) {
	for i := 0; i < chunkLength; i++ {
		if string(p.buffer[p.scanPos]) == "\n" {
			lineFinished := p.checkIfLineFinished()
			if lineFinished == true {
				if string(p.buffer[p.scanPos-3:p.scanPos+1]) == "\r\n\r\n" {
					p.state++
					p.scanPos++
					p.fieldStart = p.scanPos

					return chunkLength - i - 1, nil
				} else {
					key, value, err := getKeyValue(string(p.buffer[p.fieldStart:p.scanPos]))
					if err != nil {
						return 0, err
					}

					p.request.Headers[key] = value
					p.fieldStart = p.scanPos + 1
				}
			}
		}

		p.scanPos++
	}
	return 0, nil
}

func (p *bodyParsingMethod) getBodyParsingMethod(headers map[string]string) (parsingMethod bodyParsingMethod, getParsingMethodError error) {
	if _, contentLengthExists := headers["Content-Length"]; contentLengthExists {
		return contentLength, nil
	}
	if transferEncodingValue, transferEncodingExists := headers["Transfer-Encoding"]; transferEncodingExists {
		if transferEncodingValue != "chunked" {
			return unknown, errors.New("Unsupported 'Transfer-Encoding' method was provided.")
		}

		return transferEncoding, nil
	}
	return noContent, nil
}

// Body by 'Content-Length' header
func (p *parser) parseBodyByContentLength(chunkLength int) (parseBodyError error) {
	specifiedContentLength, contentLengthParseError := strconv.Atoi(p.request.Headers["Content-Length"])
	if contentLengthParseError != nil {
		fmt.Printf("Can't parse Content-Length value. Current string value: %s.", p.request.Headers["Content-Length"])
		return errors.New("Invalid request header 'Content-Length' value.")
	}

	if chunkLength+len(p.request.Body) > specifiedContentLength {
		fmt.Printf("Provided body (%d bytes) larger than specified 'Content-Length' (%d bytes).", chunkLength+len(p.request.Body), specifiedContentLength)
		return errors.New("Provided body larger than specified 'Content-Length'.")
	}

	p.request.Body = append(p.request.Body, p.buffer[p.scanPos+1:p.scanPos+chunkLength+1]...)
	p.scanPos += chunkLength

	if len(p.request.Body) == specifiedContentLength {
		p.state++
	}

	return nil
}

// Body by 'Transfer-Encoding: chunked' header

func (p *parser) parseBodyByChunks(receivedBytesLength int) (bytesLeft int, parseBodyError error) {
	// fieldStart points to the beginning of the chunk size.
	// scanPos points to the next unprocessed byte.
	// receivedBytesLength is kept for compatibility with the caller.
	_ = receivedBytesLength

	for {
		// 1. Find the CRLF terminating the chunk size.
		lineEnd := -1

		for i := p.scanPos; i < len(p.buffer); i++ {
			if p.buffer[i] == '\n' {
				if i == 0 || p.buffer[i-1] != '\r' {
					return 0, errors.New("Invalid chunk size line.")
				}

				lineEnd = i
				break
			}
		}

		// Chunk size line is incomplete.
		if lineEnd == -1 {
			return 0, nil
		}

		// 2. Parse hexadecimal chunk size.
		chunkSize, err := strconv.ParseUint(
			string(p.buffer[p.fieldStart:lineEnd-1]),
			16,
			64,
		)
		if err != nil {
			return 0, errors.New("Invalid chunk size.")
		}

		dataStart := lineEnd + 1

		// 3. Handle the last (zero-sized) chunk.
		if chunkSize == 0 {
			// Minimal support for the terminating CRLF.
			// Trailer fields are not supported here.
			if len(p.buffer)-dataStart < 2 {
				return 0, nil
			}

			if p.buffer[dataStart] != '\r' ||
				p.buffer[dataStart+1] != '\n' {
				return 0, errors.New("Invalid final chunk.")
			}

			p.scanPos = dataStart + 2
			p.fieldStart = p.scanPos
			p.state = stateComplete

			return len(p.buffer) - p.scanPos, nil
		}

		// 4. Wait until the complete chunk data and CRLF arrive.
		available := len(p.buffer) - dataStart

		if chunkSize > uint64(available) ||
			uint64(available)-chunkSize < 2 {
			return 0, nil
		}

		dataEnd := dataStart + int(chunkSize)

		// 5. Validate CRLF after chunk data.
		if p.buffer[dataEnd] != '\r' ||
			p.buffer[dataEnd+1] != '\n' {
			return 0, errors.New("Invalid chunk terminator.")
		}

		// 6. Append chunk data to the request body.
		p.request.Body = append(
			p.request.Body,
			p.buffer[dataStart:dataEnd]...,
		)

		// 7. Move to the next chunk size.
		p.scanPos = dataEnd + 2
		p.fieldStart = p.scanPos

		// No more buffered bytes. Wait for another TCP read.
		if p.scanPos == len(p.buffer) {
			return 0, nil
		}
	}
}

// util. returns true if line finished
func (p *parser) checkIfLineFinished() bool {
	if p.scanPos > 0 &&
		p.buffer[p.scanPos-1] == '\r' &&
		p.buffer[p.scanPos] == '\n' {
		return true
	}
	return false
}

func (p *parser) printHttpRequest() {
	r := p.request

	fmt.Printf(`
┌─ HTTP Request Parser ───────────────────────────
│ State:       %s
│ ScanPos:     %d
│ FieldStart:  %d
│ Buffer:      len=%d, cap=%d
│
│ Request
│   Method:    %q
│   Path:      %q
│   Protocol:  %q
│
│   Headers (%d)
`,
		p.state,
		p.scanPos,
		p.fieldStart,
		len(p.buffer),
		cap(p.buffer),
		r.Method,
		r.Path,
		r.Protocol,
		len(r.Headers),
	)

	keys := make([]string, 0, len(r.Headers))

	for key := range r.Headers {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		fmt.Printf("│     %-14s %q\n",
			key+":", r.Headers[key])
	}

	fmt.Printf(`│
│   Body:      %q (%d bytes)
│
│ Raw Buffer: %q
└────────────────────────────────────────

`, r.Body, len(r.Body), p.buffer)
}

// util. returns key and value from string "k:v"
func getKeyValue(str string) (key string, value string, err error) {
	dotsIndex := strings.Index(str, ":")

	if dotsIndex == -1 {
		return "", "", errors.New("invalid header: missing colon")
	}

	key = strings.TrimSpace(str[:dotsIndex])
	value = strings.TrimSpace(str[dotsIndex+1:])

	return key, value, nil
}

func (s parserState) String() string {
	switch s {
	case stateMethod:
		return "METHOD"
	case statePath:
		return "PATH"
	case stateProtocol:
		return "PROTOCOL"
	case stateHeaders:
		return "HEADERS"
	case stateBody:
		return "BODY"
	case stateComplete:
		return "COMPLETE"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

func (m bodyParsingMethod) String() string {
	switch m {
	case unknown:
		return "UNKNOWN"
	case noContent:
		return "NO CONTENT"
	case contentLength:
		return "Content-Length"
	case transferEncoding:
		return "Transfer-Encoding: chunked"
	default:
		return fmt.Sprintf("UNSUPPORTED METHOD: %d", int(m))
	}
}
