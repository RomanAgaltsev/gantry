package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// Supported values for the persistent --output/-o flag.
const (
	outputText = "text"
	outputJSON = "json"
)

// validateOutput rejects an unknown --output value so a typo (e.g. `-o josn`) fails loudly
// instead of silently rendering text and handing a script malformed output. Called once per
// command from the root PersistentPreRunE.
func validateOutput(cmd *cobra.Command) error {
	f, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	switch f {
	case outputText, outputJSON:
		return nil
	default:
		return fmt.Errorf("invalid --output %q: want %q or %q", f, outputText, outputJSON)
	}
}

// outputIsJSON reports whether the persistent --output flag selects JSON.
func outputIsJSON(cmd *cobra.Command) bool {
	f, err := cmd.Flags().GetString("output")
	if err != nil {
		return false // --output is a persistent flag; an error here is a programming bug
	}
	return f == outputJSON
}

// printJSON writes v as indented JSON to the command's stdout followed by a newline.
func printJSON(cmd *cobra.Command, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(string(b))
	return nil
}
