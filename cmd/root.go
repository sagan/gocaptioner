package cmd

import (
	"fmt"
	"os"

	"github.com/sagan/goaider/version"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "goaider",
	Short: "A CLI aider tool for AIGC " + version.Version,
	Long:  `A CLI aider tool for AIGC ` + version.Version + ".",
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}
