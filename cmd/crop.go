package cmd

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/muesli/smartcrop"
	"github.com/nfnt/resize"
	"github.com/spf13/cobra"
)

var cropCmd = &cobra.Command{
	Use:   "crop",
	Short: "Crop and resize images in a directory",
	Long:  `This command crops and resizes all images in a specified directory using smartcrop.`,
	Run: func(cmd *cobra.Command, args []string) {
		dir, _ := cmd.Flags().GetString("dir")
		output, _ := cmd.Flags().GetString("output")
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")
		force, _ := cmd.Flags().GetBool("force")

		if dir == "" {
			fmt.Println("Error: --dir flag is required.")
			_ = cmd.Help()
			os.Exit(1)
		}

		if output == "" {
			output = dir + "-crop"
		}

		if err := os.MkdirAll(output, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}

		files, err := os.ReadDir(dir)
		if err != nil {
			log.Fatalf("Failed to read directory %s: %v", dir, err)
		}

		for _, file := range files {
			if file.IsDir() || !isProcessableImage(file.Name()) {
				continue
			}

			inputPath := filepath.Join(dir, file.Name())
			outputPath := filepath.Join(output, file.Name())

			if !force {
				if _, err := os.Stat(outputPath); err == nil {
					fmt.Printf("Skipping %s, output file already exists.\n", inputPath)
					continue
				}
			}

			if err := processImageFile(inputPath, outputPath, width, height); err != nil {
				fmt.Printf("Failed to process %s: %v\n", inputPath, err)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(cropCmd)
	cropCmd.Flags().String("dir", "", "Required: Path to the image directory")
	cropCmd.Flags().String("output", "", "Optional: output dir name. default to \"<input-dir>-crop\"")
	cropCmd.Flags().Int("width", 1024, "Optional: target photo width. default: 1024.")
	cropCmd.Flags().Int("height", 1024, "Optional: target photo height. default: 1024.")
	cropCmd.Flags().Bool("force", false, "Optional bool flag. Process and generate the target output file even the same name file already exists.")
}

func isProcessableImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
	}
}

type resizer struct{}

func (r resizer) Resize(img image.Image, width, height uint) image.Image {
	return resize.Resize(width, height, img, resize.Lanczos3)
}

func processImageFile(inputPath, outputPath string, width, height int) error {
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	// Calculate crop size
	targetRatio := float64(width) / float64(height)
	imgWidth := img.Bounds().Dx()
	imgHeight := img.Bounds().Dy()
	imgRatio := float64(imgWidth) / float64(imgHeight)

	var cropWidth, cropHeight int
	if imgRatio > targetRatio {
		cropHeight = imgHeight
		cropWidth = int(float64(imgHeight) * targetRatio)
	} else {
		cropWidth = imgWidth
		cropHeight = int(float64(imgWidth) / targetRatio)
	}

	analyzer := smartcrop.NewAnalyzer(resizer{})
	topCrop, err := analyzer.FindBestCrop(img, cropWidth, cropHeight)
	if err != nil {
		return err
	}

	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}

	croppedImg := img.(subImager).SubImage(topCrop)
	resizedImg := resize.Resize(uint(width), uint(height), croppedImg, resize.Lanczos3)

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	ext := strings.ToLower(filepath.Ext(outputPath))
	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(outFile, resizedImg, nil)
	case ".png":
		err = png.Encode(outFile, resizedImg)
	default:
		return fmt.Errorf("unsupported image format: %s", ext)
	}

	if err == nil {
		fmt.Printf("Successfully cropped and resized %s to %s\n", inputPath, outputPath)
	}

	return err
}
