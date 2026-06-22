// Command gantry is a non-Kubernetes release orchestrator.
package main

import (
	"fmt"
	"os"

	"github.com/RomanAgaltsev/gantry/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "gantry:", err)
		os.Exit(1)
	}
}
