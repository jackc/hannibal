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
			ProjectPath:   viper.GetString("project_path"),
			ListenAddress: viper.GetString("http_service_address"),
			DatabaseURL:   viper.GetString("database_url"),
		})
	},
}

func init() {
	rootCmd.AddCommand(developCmd)

	developCmd.Flags().StringP("http-service-address", "a", "127.0.0.1:3000", "HTTP service address")
	viper.BindPFlag("http_service_address", developCmd.Flags().Lookup("http-service-address"))

	developCmd.Flags().StringP("database-url", "d", "", "Database URL or DSN")
	viper.BindPFlag("database_url", developCmd.Flags().Lookup("database-url"))

	developCmd.Flags().StringP("project-path", "p", ".", "Project path")
	viper.BindPFlag("project_path", developCmd.Flags().Lookup("project-path"))
}
