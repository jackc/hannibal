package cmd

import (
	"github.com/jackc/hannibal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web server",
	Run: func(cmd *cobra.Command, args []string) {
		server.Serve(&server.Config{
			ListenAddress:        viper.GetString("http_service_address"),
			DatabaseURL:          viper.GetString("database_url"),
			DatabaseSystemSchema: viper.GetString("database_system_schema"),
			DatabaseAppSchema:    viper.GetString("database_app_schema"),
		})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringP("http-service-address", "a", "127.0.0.1:3000", "HTTP service address")
	viper.BindPFlag("http_service_address", serveCmd.Flags().Lookup("http-service-address"))

	serveCmd.Flags().StringP("database-url", "d", "", "Database URL or DSN")
	viper.BindPFlag("database_url", serveCmd.Flags().Lookup("database-url"))

	serveCmd.Flags().String("database-system-schema", "hannibal_system", "Database schema for system code and data")
	viper.BindPFlag("database_system_schema", serveCmd.Flags().Lookup("database-system-schema"))

	serveCmd.Flags().String("database-app-schema", "hannibal_app", "Database schema for application code")
	viper.BindPFlag("database_app_schema", serveCmd.Flags().Lookup("database-app-schema"))

}
