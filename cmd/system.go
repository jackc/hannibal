package cmd

import (
	"github.com/spf13/cobra"
)

// systemCmd represents the db command
var systemCmd = &cobra.Command{
	Use:   "system",
	Short: "System operations",
}

func init() {
	rootCmd.AddCommand(systemCmd)
}
