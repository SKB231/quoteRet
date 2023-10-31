package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s host:port", os.Args[0])
		os.Exit(1)
	}
	name := os.Args[1] // This is the host Ip address
	tcpAddress, err := net.ResolveTCPAddr("tcp4", name)
	CheckErr(err)
	conn, err := net.DialTCP("tcp", nil, tcpAddress) // Dial up a connection: Start a connection with tcpaddress and establish it with the local address.

	CheckErr(err)

	_, err = conn.Write([]byte("HEAD / HTTP/1.0\r\n\r\n")) // From the local side
	CheckErr(err)

	result, err := io.ReadAll(conn) // Read response that has the updated the buffer for the sent HEAD request above. Until EOF is read, program won't procees beyond this point. Response is gaurenteed to be recieved since this is a TCP request and not a UDP request.
	CheckErr(err)

	fmt.Println(string(result))

	os.Exit(0)
}

func CheckErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occured: %v", err.Error())
		os.Exit(1)
	}
}
