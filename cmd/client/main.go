package main

import (
	"fmt"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:4000")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	conn.Write([]byte("POST /echo HTTP/1.1\r\n"))
	conn.Write([]byte("Host: localhost:4000\r\n"))
	// conn.Write([]byte("Content-Length: 11\r\n"))
	conn.Write([]byte("\r\n"))
	conn.Write([]byte("hello "))
	conn.Write([]byte("world"))

	buffer := make([]byte, 1024)

	readResult, readError := conn.Read(buffer)

	if readError != nil {
		fmt.Println("Error while getting server response: ", readError)
	}

	fmt.Printf("\nServer response: \n%s", string(buffer[:readResult]))

	conn.Close()
}
