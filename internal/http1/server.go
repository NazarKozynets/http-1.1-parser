package http1

import (
	"errors"
	"fmt"
	"net"
)

func StartServer() error {
	listener, err := net.Listen("tcp", ":4000")

	if err != nil {
		fmt.Println(err)
		return err
	}
	defer listener.Close()
	fmt.Println("Server is listening on port 4000...")

	for {
		conn, err := listener.Accept()

		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}

			fmt.Println("conn accept error: ", err)
			continue
		}

		p := parser{
			state:  stateMethod,
			buffer: make([]byte, 0, 512),
			request: Request{
				Headers: make(map[string]string),
			},
		}

		go handleConnection(conn, &p)
	}

	return nil
}

func handleConnection(conn net.Conn, p *parser) error {
	readBuffer := make([]byte, 16)
	defer conn.Close()

	for {
		n, err := conn.Read(readBuffer)
		if err != nil {
			fmt.Println(err)
			return err
		}

		parseErr := p.parseHttpRequest(readBuffer[:n])
		if parseErr != nil {
			fmt.Println(parseErr)
			conn.Write([]byte(string(parseErr.Error())))
			return parseErr
		}

		if p.state == stateComplete {
			conn.Write([]byte("Ok"))
			break
		}
	}

	fmt.Println("HTTP/1.1 Request was parsed")
	p.printHttpRequest()

	return nil
}
