package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/deploy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// deployCmd represents the develop command
var deployCmd = &cobra.Command{
	Use:   "deploy URL",
	Short: "Deploy to server",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("api_key", cmd.Flags().Lookup("api-key"))
		viper.BindPFlag("deploy_key", cmd.Flags().Lookup("deploy-key"))
		viper.BindPFlag("project_path", cmd.Flags().Lookup("project-path"))

		logger := current.Logger(context.Background())

		projectPath := viper.GetString("project_path")
		if projectPath == "" {
			logger.Fatal().Msg("project-path is missing")
		}

		apiKey := viper.GetString("api_key")
		if apiKey == "" {
			logger.Fatal().Msg("api-key is missing")
		}
		deployKey := viper.GetString("deploy_key")
		if deployKey == "" {
			logger.Fatal().Msg("deploy-key is missing")
		}

		err := deploy.Deploy(context.Background(), args[0], apiKey, deployKey, projectPath, nil)
		if err != nil {
			logger.Fatal().Err(err).Msg("deploy failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.Flags().String("api-key", "", "API Key")
	deployCmd.Flags().String("deploy-key", "", "Deploy Key")
	deployCmd.Flags().StringP("project-path", "p", ".", "Project path")
}
