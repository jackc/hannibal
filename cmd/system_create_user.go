package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/system"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// systemCreateUserCmd represents the system create-user command
var systemCreateUserCmd = &cobra.Command{
	Use:   "create-user",
	Short: "Create a user",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		logger := current.Logger(ctx)

		err := db.ConnectSys(ctx)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to connect to database")
		}

		username := viper.GetString("username")
		if username == "" {
			logger.Fatal().Msg("username is required")
		}
		id, err := system.CreateUser(ctx, username)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to create user")
		}

		logger.Info().Int32("id", id).Str("username", username).Msg("user created")
	},
}

func init() {
	systemCmd.AddCommand(systemCreateUserCmd)

	systemCreateUserCmd.Flags().StringP("username", "u", "", "Username")
	viper.BindPFlag("username", systemCreateUserCmd.Flags().Lookup("username"))
}
