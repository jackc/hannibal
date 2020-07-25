package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dbStatusCmd represents the db init command
var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "display database status",
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

		dbStatus, err := db.GetDBStatus(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to get database status")
		}

		if dbStatus.CurrentVersion < dbStatus.DesiredVersion {
			logger.Warn().Int32("currentVersion", dbStatus.CurrentVersion).Int32("desiredVersion", dbStatus.DesiredVersion).Msg("database needs to be upgraded")
		} else if dbStatus.CurrentVersion > dbStatus.DesiredVersion {
			logger.Warn().Int32("currentVersion", dbStatus.CurrentVersion).Int32("desiredVersion", dbStatus.DesiredVersion).Msg("database has later version than hannibal")
		} else {
			logger.Info().Int32("version", dbStatus.CurrentVersion).Msg("database is at the correct version")
		}
	},
}

func init() {
	dbCmd.AddCommand(dbStatusCmd)
}
