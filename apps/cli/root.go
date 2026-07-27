package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	apiClient *client.Client
	apiURL    string
	cfgFile   string
	version   = "dev"
	commit    = "unknown"
	date      = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI build version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive terminal UI",
	Run: func(cmd *cobra.Command, args []string) {
		m := newTUIModel(apiClient)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
			os.Exit(1)
		}
	},
}

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
		if token := viper.GetString("api_token"); token != "" {
			apiClient.SetToken(token)
		} else if token := os.Getenv("VPS_API_TOKEN"); token != "" {
			apiClient.SetToken(token)
		}
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
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(runnerCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(runbookCmd)
	rootCmd.AddCommand(approvalCmd)
	rootCmd.AddCommand(automationCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(versionCmd)
}
