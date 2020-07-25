package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/spf13/cobra"
)

// dbUpgradeCmd represents the db init command
var dbUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade database",
	Run: func(cmd *cobra.Command, args []string) {
		logger := current.Logger(context.Background())

		err := db.UpgradeDB(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to upgrade database")
		}
	},
}

func init() {
	dbCmd.AddCommand(dbUpgradeCmd)
}
