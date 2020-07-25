package cmd

import (
	"fmt"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "hannibal",
	Short: "Rapid application development",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.yak.yaml)")

	rootCmd.PersistentFlags().StringP("database-dsn", "d", "", "Primary database URL or DSN")
	viper.BindPFlag("database_dsn", rootCmd.PersistentFlags().Lookup("database-dsn"))

	rootCmd.PersistentFlags().String("database-app-schema", "hannibal_app", "Database schema for application code")
	viper.BindPFlag("database_app_schema", rootCmd.PersistentFlags().Lookup("database-app-schema"))

	rootCmd.PersistentFlags().String("database-system-dsn", "", "System database URL or DSN (uses database-dsn when empty)")
	viper.BindPFlag("database_system_dsn", rootCmd.PersistentFlags().Lookup("database-system-dsn"))

	rootCmd.PersistentFlags().String("database-system-schema", "hannibal_system", "Database schema for system code and data")
	viper.BindPFlag("database_system_schema", rootCmd.PersistentFlags().Lookup("database-system-schema"))

	rootCmd.PersistentFlags().String("database-log-dsn", "", "Log database URL or DSN (uses database-dsn when empty)")
	viper.BindPFlag("database_log_dsn", rootCmd.PersistentFlags().Lookup("database-log-dsn"))

	rootCmd.PersistentFlags().String("database-log-schema", "hannibal_log", "Database schema for logs")
	viper.BindPFlag("database_log_schema", rootCmd.PersistentFlags().Lookup("database-log-schema"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".hannibal" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".hannibal")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
