package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/spf13/cobra"
)

// dbInitCmd represents the db init command
var dbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize database",
	Run: func(cmd *cobra.Command, args []string) {
		logger := current.Logger(context.Background())

		err := db.InitDB(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize database")
		}
	},
}

func init() {
	dbCmd.AddCommand(dbInitCmd)
}
