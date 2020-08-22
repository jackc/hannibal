package cmd

import (
	"context"
	"fmt"

	"github.com/jackc/hannibal/current"
	"github.com/jackc/hannibal/db"
	"github.com/jackc/hannibal/system"
	"github.com/spf13/cobra"
)

// systemCreateDeployKeyCmd represents the system create-api-key command
var systemCreateDeployKeyCmd = &cobra.Command{
	Use:   "create-deploy-key",
	Short: "Create a deploy key for a user",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		logger := current.Logger(ctx)

		err := db.ConnectSys(ctx)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to connect to database")
		}

		username, _ := cmd.Flags().GetString("username")

		var userID int32
		err = db.Sys(ctx).QueryRow(ctx,
			fmt.Sprintf("select id from %s.users where username = $1", db.GetConfig(ctx).SysSchema),
			username,
		).Scan(&userID)
		if err != nil {
			logger.Fatal().Err(err).Str("username", username).Msg("failed to find user")
		}

		_, privateKey, err := system.CreateDeployKey(ctx, userID)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to create deploy key")
		}

		fmt.Println(privateKey)
	},
}

func init() {
	systemCmd.AddCommand(systemCreateDeployKeyCmd)

	systemCreateDeployKeyCmd.Flags().StringP("username", "u", "", "Username")
	systemCreateDeployKeyCmd.MarkFlagRequired("username")
}
