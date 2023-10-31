package main

import (
	"fmt"
	"net"
	"os"

	"github.com/SKB231/quoteRet/utils"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s host:port", os.Args[0])
	}
	name := os.Args[1] // This is the host Ip address
	tcpAddress, err := net.ResolveTCPAddr("tcp4", name)
	utils.CheckErr(err)
	conn, err := net.DialTCP("tcp", nil, tcpAddress) // Dial up a connection: Start a connection with tcpaddress and establish it with the local address.

	utils.CheckErr(err)
	response := make([]byte, 128)
	n := 0 // Initialing for later use 2 lines later
	for {
		n, err = conn.Read(response) // Read response that has the updated the buffer for the sent HEAD request above
		utils.CheckErr(err)

		if len(response) > 0 {
			fmt.Println(string(response[:n]), err)
			var userInput string
			fmt.Scanln(&userInput)
			conn.Write([]byte(userInput))

		} else {
			// Server closed connectio with us. break
			fmt.Println("Recieved nothing. Assuming server closed connection.")
			break
		}
	}
}
