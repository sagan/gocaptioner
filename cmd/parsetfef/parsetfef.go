package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/xxr3376/gtboard/pkg/ingest"

	"github.com/sagan/goaider/cmd"
	"github.com/sagan/goaider/util"
)

var (
	flagCsv string
)

// Parse an TensorBoard event file
var sttCmd = &cobra.Command{
	Use:   "parsetfef <filename>",
	Short: "Parse TensorBoard event file",
	Long:  `Parse.`,
	Args:  cobra.ExactArgs(1),
	RunE:  parsetfef,
}

func init() {
	sttCmd.Flags().StringVar(&flagCsv, "save-csv", "", "Save the parsed result to a CSV file")
	cmd.RootCmd.AddCommand(sttCmd)
}

func parsetfef(cmd *cobra.Command, args []string) error {
	r, err := ingest.NewIngester("file", args[0])
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = r.FetchUpdates(context.Background())
	if err != nil {
		return err
	}

	run := r.GetRun()

	util.PrintScalarsTable(run.Scalars)

	if flagCsv != "" {
		err := util.SaveScalarsToCSV(run.Scalars, flagCsv)
		if err != nil {
			return err
		}
	}

	return nil
}
