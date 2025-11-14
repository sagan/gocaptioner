package norfilenames

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/sagan/goaider/cmd"
)

var dirPath string
var force bool

// norfilenamesCmd represents the norfilenames command
var norfilenamesCmd = &cobra.Command{
	Use:   "norfilenames",
	Short: "Normalize filenames in a directory",
	Long: `The norfilenames command normalizes all filenames within a specified directory.
It replaces special characters (like #, $, %, etc.) in filenames with underscores (_).`,
	Run: func(cmd *cobra.Command, args []string) {
		if dirPath == "" {
			fmt.Println("Error: --dir flag is required")
			return
		}

		fmt.Printf("Normalizing filenames in directory: %s\n", dirPath)

		type renamePair struct {
			oldPath string
			newPath string
			oldName string
			newName string
		}
		var pendingRenames []renamePair

		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
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
			return
		}

		if len(pendingRenames) == 0 {
			fmt.Println("No filenames need normalization.")
			return
		}

		fmt.Println("\nPending renamings:")
		for _, rp := range pendingRenames {
			fmt.Printf("  '%s' -> '%s'\n", rp.oldName, rp.newName)
		}

		if !force {
			fmt.Print("Proceed with renaming? (y/N): ")
			var confirmation string
			fmt.Scanln(&confirmation)
			if confirmation != "y" && confirmation != "Y" {
				fmt.Println("Renaming cancelled.")
				return
			}
		}

		fmt.Println("\nPerforming renamings...")
		for _, rp := range pendingRenames {
			if err := os.Rename(rp.oldPath, rp.newPath); err != nil {
				fmt.Printf("Error renaming '%s': %v\n", rp.oldName, err)
			} else {
				fmt.Printf("Renamed '%s' to '%s'\n", rp.oldName, rp.newName)
			}
		}

		fmt.Println("Filename normalization complete.")
	},
}

func init() {
	cmd.RootCmd.AddCommand(norfilenamesCmd)
	norfilenamesCmd.Flags().StringVarP(&dirPath, "dir", "", "", "Directory to normalize filenames in")
	norfilenamesCmd.MarkFlagRequired("dir")
	norfilenamesCmd.Flags().BoolVarP(&force, "force", "", false, "Force renaming without confirmation")
}

// AddCommand adds the norfilenames command to the root command.
func AddCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(norfilenamesCmd)
}
