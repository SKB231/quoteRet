package main

import (
	"fmt"
	"net"
	"time"

	"github.com/SKB231/quoteRet/utils"
)

func initialize_server() {
	service := ":7777"
	tcpAddress, err := net.ResolveTCPAddr("tcp4", service) // resolve to localhost:7777
	utils.CheckErr(err)
	// Make a listner:
	listner, err := net.ListenTCP("tcp", tcpAddress)
	utils.CheckErr(err)
	for {
		fmt.Println("Waiting to recive connections!")
		conn, err := listner.Accept() // Accept any incoming requests.
		fmt.Println("Recieved connectionn request")
		if err != nil {
			continue
		}

		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	// Set timeout of 2 minutes
	conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	defer conn.Close() // Close the connection when we timeout or terminate due to error
	defer fmt.Println("Connection closed")
	request := make([]byte, 128) // Use this slice to handle client requests
	conn.Write([]byte("Hello how can I help you today?"))
	for {
		read_len, err := conn.Read(request)
		if err != nil {
			fmt.Println(err)
			break
		}
		if read_len == 0 {
			break // Client broke connection
		} else if string(request[:read_len]) == "timestamp" {
			conn.Write([]byte("Never gonna give you up"))
		} else {
			conn.Write([]byte("Part 2"))
		}

	}
}
