package cmd

import (
	"github.com/jackc/hannibal/develop"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// developCmd represents the develop command
var developCmd = &cobra.Command{
	Use:   "develop",
	Short: "Start development server",
	Run: func(cmd *cobra.Command, args []string) {
		develop.Develop(&develop.Config{
			ProjectPath:          viper.GetString("project_path"),
			ListenAddress:        viper.GetString("http_service_address"),
			DatabaseURL:          viper.GetString("database_url"),
			DatabaseSystemSchema: viper.GetString("database_system_schema"),
			DatabaseAppSchema:    viper.GetString("database_app_schema"),
		})
	},
}

func init() {
	rootCmd.AddCommand(developCmd)

	developCmd.Flags().StringP("http-service-address", "a", "127.0.0.1:3000", "HTTP service address")
	viper.BindPFlag("http_service_address", developCmd.Flags().Lookup("http-service-address"))

	developCmd.Flags().StringP("database-url", "d", "", "Database URL or DSN")
	viper.BindPFlag("database_url", developCmd.Flags().Lookup("database-url"))

	developCmd.Flags().String("database-system-schema", "hannibal_system", "Database schema for system code and data")
	viper.BindPFlag("database_system_schema", developCmd.Flags().Lookup("database-system-schema"))

	developCmd.Flags().String("database-app-schema", "hannibal_app", "Database schema for application code")
	viper.BindPFlag("database_app_schema", developCmd.Flags().Lookup("database-app-schema"))

	developCmd.Flags().StringP("project-path", "p", ".", "Project path")
	viper.BindPFlag("project_path", developCmd.Flags().Lookup("project-path"))
}
