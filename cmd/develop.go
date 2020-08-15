package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/develop"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// developCmd represents the develop command
var developCmd = &cobra.Command{
	Use:   "develop",
	Short: "Start development server",
	Run: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("http_service_address", cmd.Flags().Lookup("http-service-address"))
		viper.BindPFlag("project_path", cmd.Flags().Lookup("project-path"))

		logger := current.Logger(context.Background())

		current.SetSecretKeyBase("development-mode-secret-key-base")

		err := db.ConnectAll(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to connect to database")
		}

		develop.Develop(&develop.Config{
			ProjectPath:   viper.GetString("project_path"),
			ListenAddress: viper.GetString("http_service_address"),
		})
	},
}

func init() {
	rootCmd.AddCommand(developCmd)

	developCmd.Flags().StringP("http-service-address", "a", "127.0.0.1:3000", "HTTP service address")
	developCmd.Flags().StringP("project-path", "p", ".", "Project path")
}
