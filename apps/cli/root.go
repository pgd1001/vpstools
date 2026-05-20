package main

import (
	"fmt"
	"os"

	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	apiClient *client.Client
	apiURL    string
	cfgFile   string
)

var rootCmd = &cobra.Command{
	Use:   "vps",
	Short: "Secure CLI control plane for VPS fleets",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cfgFile != "" {
			viper.SetConfigFile(cfgFile)
		} else {
			configDir, err := os.UserConfigDir()
			if err != nil {
				configDir = "."
			}
			viper.AddConfigPath(configDir + "/vps-tools")
			viper.SetConfigName("config")
			viper.SetConfigType("yaml")
		}

		viper.SetEnvPrefix("vps")
		viper.AutomaticEnv()

		_ = viper.ReadInConfig()

		// API URL: flag > env > config > default
		if apiURL == "" {
			if v := viper.GetString("api_url"); v != "" {
				apiURL = v
			}
		}
		if apiURL == "" {
			apiURL = "http://localhost:8080"
		}

		apiClient = client.New(apiURL)
		if user := os.Getenv("VPS_USER"); user != "" {
			apiClient.SetUser(user)
		}
		if user := viper.GetString("user"); user != "" {
			apiClient.SetUser(user)
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(func() {})
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/vps-tools/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Control plane API URL (or set VPS_API_URL env)")
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(runnerCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(auditCmd)
}
