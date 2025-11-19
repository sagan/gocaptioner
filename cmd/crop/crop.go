package crop

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/muesli/smartcrop"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/sagan/goaider/cmd"
	"github.com/spf13/cobra"
)

// Flag variables to store command line arguments
var (
	flagDir       string
	flagOutputDir string
	flagWidth     int
	flagHeight    int
	flagForce     bool
)

var cropCmd = &cobra.Command{
	Use:   "crop",
	Short: "Crop and resize images in a directory",
	Long:  `This command crops and resizes all images in a specified directory using smartcrop.`,
	RunE:  crop,
}

func init() {
	cmd.RootCmd.AddCommand(cropCmd)

	// Bind flags to variables using StringVar, IntVar, BoolVar
	cropCmd.Flags().StringVar(&flagDir, "dir", "", "Required: Path to the image directory")
	cropCmd.Flags().StringVar(&flagOutputDir, "output", "", "Optional: output dir name. default to \"<input-dir>-crop\"")
	cropCmd.Flags().IntVar(&flagWidth, "width", 1024, "Optional: target photo width. default: 1024.")
	cropCmd.Flags().IntVar(&flagHeight, "height", 1024, "Optional: target photo height. default: 1024.")
	cropCmd.Flags().BoolVar(&flagForce, "force", false, "Optional: Process and generate the target output file even if the file already exists.")
	cropCmd.MarkFlagRequired("dir")
}

func crop(cmd *cobra.Command, args []string) error {
	// Logic: specific output directory calculation
	finalOutput := flagOutputDir
	if finalOutput == "" {
		absDir, err := filepath.Abs(flagDir)
		if err != nil {
			return fmt.Errorf("failed to resolve path %s: %w", flagDir, err)
		}
		finalOutput = absDir + "-crop"
	}

	if err := os.MkdirAll(finalOutput, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	files, err := os.ReadDir(flagDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", flagDir, err)
	}

	errorCnt := 0
	for _, file := range files {
		if file.IsDir() || !isProcessableImage(file.Name()) {
			continue
		}

		inputPath := filepath.Join(flagDir, file.Name())
		outputPath := filepath.Join(finalOutput, file.Name())

		if !flagForce {
			if _, err := os.Stat(outputPath); err == nil {
				fmt.Printf("Skipping %s, output file already exists.\n", inputPath)
				continue
			}
		}

		if err := processImageFile(inputPath, outputPath, flagWidth, flagHeight); err != nil {
			fmt.Printf("Failed to process %s: %v\n", inputPath, err)
			errorCnt++
		}
	}
	if errorCnt > 0 {
		return fmt.Errorf("%d errors", errorCnt)
	}
	return nil
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
		return fmt.Errorf("failed to rewind file: %w", err)
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
