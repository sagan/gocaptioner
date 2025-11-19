package sovitsgenlist

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sagan/goaider/cmd"
)

var (
	flagDir     string
	flagLang    string
	flagForce   bool
	flagSpeaker string
	flagOutput  string
)

var genlistCmd = &cobra.Command{
	Use:   "sovits-genlist",
	Short: "Generates a GPT-SoVITS dataset annotation sovits.list file",
	Long: `The sovits-genlist command generates a dataset annotation sovits.list file
used by GPT-SoVITS (a voice synthesis and cloning model).

It reads all "<filename>.wav" files and corresponding "<filename>.txt"
transcription files from a specified directory, then generates a
"sovits.list" file in that directory.

Each line in the generated .list file will have the format:
audio_filename|speaker|language|text

Example:
foo1.wav|foo|en|I have a dream

Notes:
- Only include a wav file record in sovits.list file if a corresponding .txt
  transcription file exists.
- If a .txt file has multiple lines, replace new line breaks (\r\n / \n)
  with a single space.`,
	RunE: runSovitsGenlist,
}

func init() {
	genlistCmd.Flags().StringVarP(&flagDir, "dir", "", "", "Required. Directory containing audio & transcription files.")
	genlistCmd.Flags().StringVarP(&flagOutput, "output", "", "sovits.list", `Output filename in target dir. Set to "-" to output to stdout`)
	genlistCmd.Flags().StringVarP(&flagLang, "lang", "", "", "Required. The language spoken in the audio files: zh | ja | en | ko | yue.")
	genlistCmd.Flags().BoolVarP(&flagForce, "force", "", false, `Force re-generate "sovits.list" file even if it already exists.`)
	genlistCmd.Flags().StringVarP(&flagSpeaker, "speaker", "", "", "Required. Speaker name.")

	genlistCmd.MarkFlagRequired("dir")
	genlistCmd.MarkFlagRequired("lang")
	genlistCmd.MarkFlagRequired("speaker")
	cmd.RootCmd.AddCommand(genlistCmd)
}

func runSovitsGenlist(cmd *cobra.Command, args []string) error {
	var err error
	// Validate language
	validLangs := map[string]bool{"zh": true, "ja": true, "en": true, "ko": true, "yue": true}
	if !validLangs[flagLang] {
		return fmt.Errorf("invalid language: %q. Must be one of: zh, ja, en, ko, yue", flagLang)
	}

	// Get absolute path for the directory
	absDirPath, err := filepath.Abs(flagDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for directory %q: %w", flagDir, err)
	}

	var outputFilePath string
	if flagOutput != "-" {
		outputFilePath = filepath.Join(absDirPath, flagOutput)
		// Check if output file exists and if force flag is not set
		if _, err := os.Stat(outputFilePath); err == nil && !flagForce {
			return fmt.Errorf("output file %q already exists. Use --force to overwrite", outputFilePath)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to check existence of output file %q: %w", outputFilePath, err)
		}
	} else {
		outputFilePath = "-"
	}

	// Read directory contents
	entries, err := os.ReadDir(absDirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %q: %w", absDirPath, err)
	}

	var listLines []string
	wavFiles := make(map[string]struct{}) // To keep track of found wav files

	// First pass: collect all .wav files
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".wav") {
			baseName := strings.TrimSuffix(entry.Name(), ".wav")
			wavFiles[baseName] = struct{}{}
		}
	}

	// Second pass: process .txt files that have corresponding .wav files
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txt") {
			baseName := strings.TrimSuffix(entry.Name(), ".txt")

			if _, exists := wavFiles[baseName]; exists {
				txtFilePath := filepath.Join(absDirPath, entry.Name())
				content, err := os.ReadFile(txtFilePath)
				if err != nil {
					log.Printf("Warning: Failed to read transcription file %q: %v. Skipping.", txtFilePath, err)
					continue
				}

				// Replace newlines with spaces
				text := strings.ReplaceAll(string(content), "\r\n", " ")
				text = strings.ReplaceAll(text, "\n", " ")
				text = strings.TrimSpace(text) // Trim leading/trailing spaces

				// Format the line
				line := fmt.Sprintf("%s.wav|%s|%s|%s", baseName, flagSpeaker, flagLang, text)
				listLines = append(listLines, line)
			}
		}
	}

	if len(listLines) == 0 {
		return fmt.Errorf("no valid wav files found")
	}

	var outputFile *os.File
	if outputFilePath != "-" {
		// Write to output file
		outputFile, err = os.Create(outputFilePath)
		if err != nil {
			return fmt.Errorf("failed to create output file %q: %w", outputFilePath, err)
		}
		defer outputFile.Close()
	} else {
		outputFile = os.Stdout
	}

	writer := bufio.NewWriter(outputFile)
	for _, line := range listLines {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			return fmt.Errorf("failed to write line to output file: %w", err)
		}
	}
	writer.Flush()

	log.Printf("Successfully generated GPT-SoVITS list file: %q", outputFilePath)
	return nil
}
