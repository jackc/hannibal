package cmd

import (
	"context"
	"fmt"

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

		fmt.Println(viper.GetString("project_path"))
		pkg, err := deploy.NewPackage(viper.GetString("project_path"))
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to load project")
		}

		apiKey := viper.GetString("api_key")
		if apiKey == "" {
			logger.Fatal().Msg("api-key is missing")
		}
		deployKey := viper.GetString("deploy_key")
		if deployKey == "" {
			logger.Fatal().Msg("deploy-key is missing")
		}

		deployer := &deploy.Deployer{
			URL:       args[0],
			APIKey:    apiKey,
			DeployKey: deployKey,
		}

		err = deployer.Deploy(context.Background(), pkg)
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
