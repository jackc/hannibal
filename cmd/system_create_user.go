package cmd

import (
	"context"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/system"
	"github.com/spf13/cobra"
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

		username, _ := cmd.Flags().GetString("username")
		_, err = system.CreateUser(ctx, username)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to create user")
		}
	},
}

func init() {
	systemCmd.AddCommand(systemCreateUserCmd)

	systemCreateUserCmd.Flags().StringP("username", "u", "", "Username")
	systemCreateUserCmd.MarkFlagRequired("username")
}
