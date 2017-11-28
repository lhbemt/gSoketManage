package mian

import (
	"../socketmanage"
	"fmt"
)

func main() {
	server := socketmanage.New()
	err := server.Work()
	if err != nil {
		fmt.Print("start server error")
	}

	select {}
}
