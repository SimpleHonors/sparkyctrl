package main

import (
	"os"

	"github.com/SimpleHonors/sparkyctrl/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
