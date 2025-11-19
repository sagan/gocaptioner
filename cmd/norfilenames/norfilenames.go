package norfilenames

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/sagan/goaider/cmd"
)

var (
	flagDir   string
	flagForce bool
)

// norfilenamesCmd represents the norfilenames command
var norfilenamesCmd = &cobra.Command{
	Use:   "norfilenames",
	Short: "Normalize filenames in a directory",
	Long: `The norfilenames command normalizes all filenames within a specified directory.
It replaces special characters (like #, $, %, etc.) in filenames with underscores (_).`,
	RunE: norfilenames,
}

func init() {
	cmd.RootCmd.AddCommand(norfilenamesCmd)
	norfilenamesCmd.Flags().StringVarP(&flagDir, "dir", "", "", "Directory to normalize filenames in")
	norfilenamesCmd.Flags().BoolVarP(&flagForce, "force", "", false, "Force renaming without confirmation")
	norfilenamesCmd.MarkFlagRequired("dir")
}

func norfilenames(cmd *cobra.Command, args []string) error {
	fmt.Printf("Normalizing filenames in directory: %s\n", flagDir)

	type renamePair struct {
		oldPath string
		newPath string
		oldName string
		newName string
	}
	var pendingRenames []renamePair

	err := filepath.Walk(flagDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			dir := filepath.Dir(path)
			oldName := info.Name()

			// Normalize the filename: replace special characters with '_'
			// Special char: ASCII char and not in [-_.a-zA-Z0-9]
			re := regexp.MustCompile(`[\x00-\x2C\x2F\x3A-\x40\x5B-\x5E\x60\x7B-\x7F]`)
			newName := re.ReplaceAllString(oldName, "_")

			if oldName != newName {
				newPath := filepath.Join(dir, newName)
				pendingRenames = append(pendingRenames, renamePair{oldPath: path, newPath: newPath, oldName: oldName, newName: newName})
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking the directory: %v\n", err)
		return nil
	}

	if len(pendingRenames) == 0 {
		fmt.Println("No filenames need normalization.")
		return nil
	}

	fmt.Println("\nPending renamings:")
	for _, rp := range pendingRenames {
		fmt.Printf("  '%s' -> '%s'\n", rp.oldName, rp.newName)
	}

	if !flagForce {
		fmt.Print("Proceed with renaming? (y/N): ")
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation != "y" && confirmation != "Y" && confirmation != "yes" && confirmation != "YES" {
			fmt.Printf("Renaming cancelled.\n")
			return nil
		}
	}

	fmt.Printf("\n")
	fmt.Printf("Performing renamings...\n")
	errorCnt := 0
	for _, rp := range pendingRenames {
		if err := os.Rename(rp.oldPath, rp.newPath); err != nil {
			fmt.Printf("Error renaming %q: %v\n", rp.oldName, err)
			errorCnt++
		} else {
			fmt.Printf("Renamed %q to %q\n", rp.oldName, rp.newName)
		}
	}

	fmt.Printf("Filename normalization complete.\n")
	if errorCnt > 0 {
		return fmt.Errorf("%d errors", errorCnt)
	}
	return nil
}

// AddCommand adds the norfilenames command to the root command.
func AddCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(norfilenamesCmd)
}
