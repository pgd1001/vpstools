package main

import (
	"fmt"
	"os"

	"github.com/pgd1001/svrtools/packages/sdk-go/client"
	"github.com/spf13/cobra"
)

var aiCmd = &cobra.Command{Use: "ai", Short: "Run safe, read-only AI analysis"}
var aiAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyse an execution or supplied evidence without changing infrastructure",
	Run: func(cmd *cobra.Command, args []string) {
		question, _ := cmd.Flags().GetString("question")
		executionID, _ := cmd.Flags().GetString("execution")
		resp, err := apiClient.AnalyzeAI(client.AIAnalysisRequest{Question: question, ExecutionID: executionID})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return
		}
		fmt.Printf("Analysis %s [%s, read-only]\n\n%s\n", resp.AnalysisID, resp.Model, resp.Text)
	},
}

func init() {
	aiAnalyzeCmd.Flags().String("question", "", "Question for the read-only analyst")
	aiAnalyzeCmd.Flags().String("execution", "", "Use redacted output from this execution as evidence")
	_ = aiAnalyzeCmd.MarkFlagRequired("question")
	aiCmd.AddCommand(aiAnalyzeCmd)
}
