package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dbUpgradeCmd represents the db init command
var dbUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade database",
	Run: func(cmd *cobra.Command, args []string) {
		logger := current.Logger(context.Background())

		dbConfig := &db.Config{
			AppConnString: viper.GetString("database_dsn"),
			AppSchema:     viper.GetString("database_app_schema"),

			SysConnString: viper.GetString("database_system_dsn"),
			SysSchema:     viper.GetString("database_system_schema"),

			LogConnString: viper.GetString("database_log_dsn"),
			LogSchema:     viper.GetString("database_log_schema"),
		}
		dbConfig.SetDerivedDefaults()

		err := db.Connect(context.Background(), dbConfig)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to connect to database")
		}

		err = db.UpgradeDB(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to upgrade database")
		}
	},
}

func init() {
	dbCmd.AddCommand(dbUpgradeCmd)
}
