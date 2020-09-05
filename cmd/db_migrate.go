package cmd

import (
	"context"
	"path/filepath"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// dbMigrateCmd represents the migrate command
var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate database",
	Run: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("project_path", cmd.Flags().Lookup("project-path"))

		logger := current.Logger(context.Background())

		projectPath := viper.GetString("project_path")
		if projectPath == "" {
			logger.Fatal().Msg("project-path is missing")
		}

		err := db.MigrateDB(context.Background(), filepath.Join(projectPath, "migration"))
		if err != nil {
			logger.Fatal().Err(err).Msg("migrate failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(dbMigrateCmd)

	dbMigrateCmd.Flags().StringP("project-path", "p", ".", "Project path")
}
