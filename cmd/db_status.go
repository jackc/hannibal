package cmd

import (
	"context"
	"path/filepath"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dbStatusCmd represents the migrate command
var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display application migration status",
	Run: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("project_path", cmd.Flags().Lookup("project-path"))

		logger := current.Logger(context.Background())

		projectPath := viper.GetString("project_path")
		if projectPath == "" {
			logger.Fatal().Msg("project-path is missing")
		}

		dbStatus, err := db.GetAppDBStatus(context.Background(), filepath.Join(projectPath, "migration"))
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

	dbStatusCmd.Flags().StringP("project-path", "p", ".", "Project path")
}
