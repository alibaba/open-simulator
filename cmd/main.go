package main

import (
	"fmt"
	"os"

	"github.com/alibaba/open-simulator/cmd/simon"
)

func main() {
	cmd := simon.NewSimonCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Printf("main | start with error: %s", err.Error())
		os.Exit(1)
	}
}
