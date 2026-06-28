// Command gantry is a non-Kubernetes release orchestrator.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/RomanAgaltsev/gantry/internal/cli"
)

func main() {
	err := cli.Execute()
	if err != nil && !errors.Is(err, cli.ErrDriftDetected) {
		fmt.Fprintln(os.Stderr, "gantry:", err)
	}
	os.Exit(cli.ExitCode(err))
}
