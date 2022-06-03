package main

import (
	"os"

	"github.com/alibaba/open-simulator/cmd/simon"
	"github.com/pterm/pterm"
)

func main() {
	cmd := simon.NewSimonCommand()
	if err := cmd.Execute(); err != nil {
		pterm.FgRed.Printf("start with error: %s", err.Error())
		os.Exit(1)
	}
}
