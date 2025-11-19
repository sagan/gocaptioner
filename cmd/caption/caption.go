package caption

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sagan/goaider/cmd"
	"github.com/sagan/goaider/constants"
)

// --- Structs for Gemini API Request ---

type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// --- Structs for Gemini API Response ---

type GeminiResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

// --- API and Program Constants ---

const (
	// This prompt is optimized for LoRa training captions
	captionPrompt = `Generate a simple, comma-separated caption for this image, optimized for LoRa training.

RULES:
1.  Focus ONLY on the main subject.
2.  Describe visible attributes: clothing (e.g., "pink jacket"), hairstyle (e.g., "ponytail"), pose (e.g., "crouching", "standing"), and expression (e.g., "smiling").
3.  You can describe an object that the main subject is interacting with (e.g., "holding a toy").

CRITICAL:
* DO NOT use general category words like "girl", "boy", "child", "woman", "man", or "person".
* DO NOT describe the background, environment, or location (e.g., AVOID "in a room", "child's room", "indoor", "outside", "at home").
* DO NOT describe artistic style, lighting, camera quality, or effects.

Good example: "pink puffer jacket, ponytail, hair clips, crouching, holding toy".

Bad example: "young girl, pink puffer jacket, fur collar, black pants, slippers, pink bunny hair clips, ponytail, pink bobbles, crouching, holding a pink plastic toy, child's room, pink desk, pink chair, toys, curtains, wooden floor".
"
`

	maxRetries = 3 // Number of retries for API calls
)

// Flag variables to store command line arguments
var (
	flagDir      string
	flagForce    bool
	flagIdentity string
	flagModel    string
)

var captionCmd = &cobra.Command{
	Use:   "caption",
	Short: "Generate captions for images in a directory",
	Long:  `This command generates captions for all images in a specified directory using the Gemini API.`,
	RunE:  caption,
}

func init() {
	cmd.RootCmd.AddCommand(captionCmd)
	// Refactored to use Var functions to bind flags to package-level variables
	captionCmd.Flags().StringVar(&flagDir, "dir", "", "Required: Path to the image directory")
	captionCmd.Flags().BoolVar(&flagForce, "force", false, "Optional: Force re-generation of all captions, even if .txt files exist")
	captionCmd.Flags().StringVar(&flagIdentity, "identity", "", "Optional: The trigger word (e.g., 'foobar' or 'photo of foobar') to prepend to each caption")
	captionCmd.Flags().StringVarP(&flagModel, "model", "", constants.DEFAULT_GEMINI_MODEL, "The model to use for captioning")

	captionCmd.MarkFlagRequired("dir")
}

func caption(cmd *cobra.Command, args []string) error {
	// 1. Get API Key from environment
	apiKey := os.Getenv(constants.ENV_GEMINI_API_KEY)
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	// 3. Read the specified directory
	files, err := os.ReadDir(flagDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", flagDir, err)
	}

	fmt.Printf("Starting captioning for images in: %s\n", flagDir)
	if flagForce {
		fmt.Printf("FORCE flag set: Re-generating all captions.\n")
	}
	if flagIdentity != "" {
		fmt.Printf("IDENTITY set: Prepending %q to all new captions.\n", flagIdentity)
	}

	// Create an HTTP client with a timeout
	client := &http.Client{Timeout: 45 * time.Second}

	errorCnt := 0
	// 4. Loop over all files and process images
	for _, file := range files {
		if file.IsDir() || !isImageFile(file.Name()) {
			continue // Skip directories and non-image files
		}

		fullPath := filepath.Join(flagDir, file.Name())

		// processImage does all the work: API call, retries, and file saving
		err := processImage(client, fullPath, apiKey, flagForce, flagIdentity)
		if err != nil {
			fmt.Printf("Processing %s: ❌ FAILED (%v)\n", file.Name(), err)
			errorCnt++
		}
	}
	fmt.Printf("Captioning complete.\n")
	if errorCnt > 0 {
		return fmt.Errorf("%d errors", errorCnt)
	}
	return nil
}

/**
 * processImage handles the full logic for a single image:
 * 1. Checks if caption file exists (and skips if -force is not set)
 * 2. Reads the image file
 * 3. Encodes it to base64
 * 4. Calls the Gemini API (with retries)
 * 5. Parses the response
 * 6. Prepends identity (if provided)
 * 7. Saves the caption to a .txt file
 */
func processImage(client *http.Client, imagePath string, apiKey string, force bool, identity string) error {
	// 1. Check for existing .txt file before doing any work
	baseName := filepath.Base(imagePath)
	ext := filepath.Ext(baseName)
	txtFileName := strings.TrimSuffix(baseName, ext) + ".txt"
	txtPath := filepath.Join(filepath.Dir(imagePath), txtFileName)

	if !force {
		if _, err := os.Stat(txtPath); err == nil {
			// File exists, skip processing
			fmt.Printf("Processing %s: ⏩ SKIPPED (caption already exists)\n", baseName)
			return nil
		}
	}

	fmt.Printf("Processing %s: ⏳ GENERATING...\n", baseName)

	// 2. Read image file and encode to base64
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return fmt.Errorf("failed to read image: %w", err)
	}
	base64Image := base64.StdEncoding.EncodeToString(imageData)
	mimeType := getMimeType(imagePath)

	// 3. Construct the API request payload
	payload := GeminiRequest{
		Contents: []Content{
			{
				Role: "user",
				Parts: []Part{
					{Text: captionPrompt}, // The prompt to the model
					{
						InlineData: &InlineData{ // The image data
							MimeType: mimeType,
							Data:     base64Image,
						},
					},
				},
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	apiUrl := fmt.Sprintf("%s%s:generateContent?key=%s", constants.GEMINI_API_URL, flagModel, apiKey)
	var geminiResp GeminiResponse
	var resp *http.Response
	var reqErr error
	delay := 2 * time.Second // Initial retry delay

	// 4. API Call with simple exponential backoff
	for range maxRetries {
		req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonPayload))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, reqErr = client.Do(req)

		// If there's a network error, retry
		if reqErr != nil {
			fmt.Printf("  ...network error (%v), retrying in %v\n", reqErr, delay)
			time.Sleep(delay)
			delay *= 2 // Double the delay for next retry
			continue
		}

		// Check for 429 (Throttling) or 5xx (Server Error) and retry
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			fmt.Printf("  ...API error (%s), retrying in %v\n", resp.Status, delay)
			if resp.Body != nil {
				resp.Body.Close() // Must close body before retrying
			}
			time.Sleep(delay)
			delay *= 2
			continue
		}

		// Any other non-200 status code is a non-retryable error
		if resp.StatusCode != http.StatusOK {
			break // Exit the loop to handle the error below
		}

		// Try to decode the response. If it's empty, we might want to retry.
		if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
			if resp.Body != nil {
				resp.Body.Close()
			}
			return fmt.Errorf("failed to decode API response: %w", err)
		}
		resp.Body.Close() // Close body after successful decode

		// If the response is empty, retry
		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 || geminiResp.Candidates[0].Content.Parts[0].Text == "" {
			fmt.Printf("  ...API returned empty caption, retrying in %v\n", delay)
			time.Sleep(delay)
			delay *= 2
			continue
		}

		// If we got a valid response, break the loop
		break
	}

	// If all retries failed on a network error
	if reqErr != nil {
		return fmt.Errorf("all retries failed: %w", reqErr)
	}

	// Handle non-OK, non-retryable status codes after the loop
	if resp != nil && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %s", resp.Status)
	}

	// 5. Extract the caption text (already decoded in the loop)
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 || geminiResp.Candidates[0].Content.Parts[0].Text == "" {
		return fmt.Errorf("no caption generated (empty response from API)")
	}
	caption := geminiResp.Candidates[0].Content.Parts[0].Text

	// 6. Prepend identity if provided
	finalCaption := strings.TrimSpace(caption) // Clean up any extra whitespace
	if identity != "" {
		finalCaption = identity + ", " + finalCaption
	}

	// 7. Save the caption to a .txt file
	err = os.WriteFile(txtPath, []byte(finalCaption), 0644)
	if err != nil {
		return fmt.Errorf("failed to write caption file: %w", err)
	}

	fmt.Printf("Processing %s: ✅ SUCCESS\n", baseName)
	return nil
}

// isImageFile checks if a filename has a common image extension
func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

// getMimeType determines the MIME type from the file extension
func getMimeType(imagePath string) string {
	ext := strings.ToLower(filepath.Ext(imagePath))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		// A safe default
		return "application/octet-stream"
	}
}
