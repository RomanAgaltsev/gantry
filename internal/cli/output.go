package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// outputIsJSON reports whether the persistent --output flag selects JSON.
func outputIsJSON(cmd *cobra.Command) bool {
	f, err := cmd.Flags().GetString("output")
	if err != nil {
		return false // --output is a persistent flag; an error here is a programming bug
	}
	return f == "json"
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
