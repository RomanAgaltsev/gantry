package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/logging"
)

func TestRoot_InjectsLoggerIntoContext(t *testing.T) {
	root := NewRootCmd()
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	root.SetOut(&bytes.Buffer{})

	// A probe subcommand added only in the test, exercising the context logger.
	probe := &cobra.Command{
		Use: "probe",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logging.From(cmd.Context()).Info("probe-ran", "ok", true)
			return nil
		},
	}
	root.AddCommand(probe)
	root.SetArgs([]string{"probe", "--log-format", "json"})

	require.NoError(t, root.Execute())
	require.Contains(t, errBuf.String(), `"msg":"probe-ran"`) // logged as JSON to stderr
}

func TestRoot_HasLoggingFlags(t *testing.T) {
	root := NewRootCmd()
	require.NotNil(t, root.PersistentFlags().Lookup("log-format"))
	require.NotNil(t, root.PersistentFlags().Lookup("log-level"))
}
