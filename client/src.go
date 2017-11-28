package main

import (
	"net"
	"fmt"
	"io"
	"os"
)

func handleconn(f net.Conn) {
	for {
		io.Copy(os.Stdout, f)
	}
}

func handlewrite(f net.Conn) {
	for {
		io.Copy(f, os.Stdin)
	}
}

func main() {
	conn,err := net.Dial("tcp", ":8080")
	if err != nil {
		fmt.Printf("conn error: %v\n", err)
		return
	}
	go handleconn(conn)
	go handlewrite(conn)

	select {}
}
