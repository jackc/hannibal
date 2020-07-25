package cmd

import (
	"context"
	"os"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/server"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web server",
	Run: func(cmd *cobra.Command, args []string) {
		log := zerolog.New(os.Stdout).With().
			Timestamp().
			Logger()
		current.SetLogger(&log)

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
			log.Fatal().Err(err).Msg("failed to connect to database")
		}

		server.Serve(&server.Config{
			ListenAddress: viper.GetString("http_service_address"),
		})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringP("http-service-address", "a", "127.0.0.1:3000", "HTTP service address")
	viper.BindPFlag("http_service_address", serveCmd.Flags().Lookup("http-service-address"))
}
