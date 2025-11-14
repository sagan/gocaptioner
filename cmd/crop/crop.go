package crop

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging" // ADD THIS
	"github.com/muesli/smartcrop"
	"github.com/rwcarlsen/goexif/exif" // ADD THIS
	"github.com/sagan/gocaptioner/cmd"
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
			absDir, err := filepath.Abs(dir)
			if err != nil {
				log.Fatalf("Failed to resolve path %s: %v", dir, err)
			}
			output = absDir + "-crop"
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
	cmd.RootCmd.AddCommand(cropCmd)
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
	// Use the new imaging library's Resize function
	return imaging.Resize(img, int(width), int(height), imaging.Lanczos)
}

func processImageFile(inputPath, outputPath string, width, height int) error {
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// -----------------------------------------------------------------
	// START: EXIF Orientation Fix
	// -----------------------------------------------------------------

	// 1. Read EXIF data first
	x, err := exif.Decode(file)
	var orientation int
	if err == nil { // If EXIF data exists
		tag, err := x.Get(exif.Orientation)
		if err == nil { // If Orientation tag exists
			orientation, _ = tag.Int(0) // Get the orientation value
		}
	}

	// 2. Rewind the file to read it again for image decoding
	_, err = file.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to rewind file: %v", err)
	}

	// 3. Decode the image (and get its format)
	img, imgFormat, err := image.Decode(file)
	if err != nil {
		return err
	}

	// 4. Apply rotation IF it's a JPEG and has an orientation tag
	if imgFormat == "jpeg" && orientation > 1 {
		img = applyExifOrientation(img, orientation)
	}

	// -----------------------------------------------------------------
	// END: EXIF Orientation Fix
	// -----------------------------------------------------------------

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

	// Use imaging.Resize for the final resize
	resizedImg := imaging.Resize(croppedImg, width, height, imaging.Lanczos)

	// -----------------------------------------------------------------
	// START: Corrected Save Logic
	// -----------------------------------------------------------------

	// REMOVED:
	// outFile, err := os.Create(outputPath)
	// ...
	// defer outFile.Close()

	// Use imaging.Save, passing the image and the *path string*.
	ext := strings.ToLower(filepath.Ext(outputPath))
	switch ext {
	case ".jpg", ".jpeg":
		// Correct signature: imaging.Save(image, path, ...options)
		err = imaging.Save(resizedImg, outputPath, imaging.JPEGQuality(95))
	case ".png":
		// Correct signature: imaging.Save(image, path, ...options)
		err = imaging.Save(resizedImg, outputPath, imaging.PNGCompressionLevel(png.DefaultCompression))
	default:
		return fmt.Errorf("unsupported image format: %s", ext)
	}

	// -----------------------------------------------------------------
	// END: Corrected Save Logic
	// -----------------------------------------------------------------

	if err == nil {
		fmt.Printf("Successfully cropped and resized %s to %s\n", inputPath, outputPath)
	}

	return err
}

// applyExifOrientation checks for an EXIF orientation tag and rotates the image accordingly.
func applyExifOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2: // F: Horizontal Flip
		return imaging.FlipH(img)
	case 3: // R180: Rotate 180
		return imaging.Rotate180(img)
	case 4: // FV: Vertical Flip
		return imaging.FlipV(img)
	case 5: // T: Transpose (FlipH + R270)
		return imaging.Transpose(img)
	case 6: // R270: Rotate 270 (or 90 clockwise)
		return imaging.Rotate270(img)
	case 7: // TV: Transverse (FlipV + R270)
		return imaging.Transverse(img)
	case 8: // R90: Rotate 90 (or 270 clockwise)
		return imaging.Rotate90(img)
	default: // 1 or unknown
		return img
	}
}
