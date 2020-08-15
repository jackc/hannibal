package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web server",
	Run: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("http_service_address", cmd.Flags().Lookup("http-service-address"))
		viper.BindPFlag("app_path", cmd.Flags().Lookup("app-path"))
		viper.BindPFlag("secret_key_base", cmd.Flags().Lookup("secret-key-base"))

		logger := current.Logger(context.Background())

		secretKeyBase := viper.GetString("secret_key_base")
		if secretKeyBase == "" {
			logger.Warn().Msg("missing secret_key_base")
			var err error
			secretKeyBase, err = server.GenerateSecretKeyBase()
			if err != nil {
				logger.Fatal().Err(err).Msg("generate random secret_key_base failed")
			}
			logger.Info().Str("secret_key_base", secretKeyBase).Msg("using random secret_key_base")
		}
		if len(secretKeyBase) < 64 {
			logger.Fatal().Msg("secret_key_base must be at least 64 characters")
		}
		current.SetSecretKeyBase(secretKeyBase)

		err := db.ConnectAll(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to connect to database")
		}

		server.Serve(&server.Config{
			ListenAddress: viper.GetString("http_service_address"),
			AppPath:       viper.GetString("app_path"),
		})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringP("http-service-address", "a", "127.0.0.1:3000", "HTTP service address")
	serveCmd.Flags().StringP("app-path", "p", ".", "Application path")
	serveCmd.Flags().String("secret-key-base", "", "Secret key base")
}
