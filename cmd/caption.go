package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
	// Use the correct model for image understanding
	apiModel   = "gemini-1.5-flash-preview-0514"
	apiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

	// This prompt is optimized for LoRa training captions
	captionPrompt = "Generate a simple, comma-separated caption for this image for LoRa training. Describe only the main subject, their clothing, their pose/action, and the background setting. Do NOT describe artistic style, camera angles, lighting, or blurry effects."

	maxRetries = 3 // Number of retries for API calls
)

var captionCmd = &cobra.Command{
	Use:   "caption",
	Short: "Generate captions for images in a directory",
	Long:  `This command generates captions for all images in a specified directory using the Gemini API.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Get API Key from environment
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			log.Fatal("GEMINI_API_KEY environment variable not set")
		}

		// 2. Get flags
		dirFlag, _ := cmd.Flags().GetString("dir")
		forceFlag, _ := cmd.Flags().GetBool("force")
		identityFlag, _ := cmd.Flags().GetString("identity")

		if dirFlag == "" {
			fmt.Println("Error: -dir flag is required.")
			_ = cmd.Help()
			os.Exit(1)
		}
		dirPath := dirFlag

		// 3. Read the specified directory
		files, err := os.ReadDir(dirPath)
		if err != nil {
			log.Fatalf("Failed to read directory %s: %v", dirPath, err)
		}

		fmt.Printf("Starting captioning for images in: %s\n", dirPath)
		if forceFlag {
			fmt.Println("FORCE flag set: Re-generating all captions.")
		}
		if identityFlag != "" {
			fmt.Printf("IDENTITY set: Prepending \"%s\" to all new captions.\n", identityFlag)
		}

		// Create an HTTP client with a timeout
		client := &http.Client{Timeout: 45 * time.Second}

		// 4. Loop over all files and process images
		for _, file := range files {
			if file.IsDir() || !isImageFile(file.Name()) {
				continue // Skip directories and non-image files
			}

			fullPath := filepath.Join(dirPath, file.Name())

			// processImage does all the work: API call, retries, and file saving
			err := processImage(client, fullPath, apiKey, forceFlag, identityFlag)
			if err != nil {
				fmt.Printf("Processing %s: ❌ FAILED (%v)\n", file.Name(), err)
			}
		}
		fmt.Println("Captioning complete.")
	},
}

func init() {
	rootCmd.AddCommand(captionCmd)
	captionCmd.Flags().String("dir", "", "Required: Path to the image directory")
	captionCmd.Flags().Bool("force", false, "Optional: Force re-generation of all captions, even if .txt files exist")
	captionCmd.Flags().String("identity", "", "Optional: The trigger word (e.g., 'kongrongjin_3y') to prepend to each caption")
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

	apiUrl := fmt.Sprintf("%s%s:generateContent?key=%s", apiBaseURL, apiModel, apiKey)

	var resp *http.Response
	var reqErr error
	delay := 2 * time.Second // Initial retry delay

	// 4. API Call with simple exponential backoff
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonPayload))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, reqErr = client.Do(req)

		// If there's a network error, retry
		if reqErr != nil {
			fmt.Printf("  ...network error (%v), retrying in %v\n", reqErr, delay)
			time.Sleep(delay)
			delay *= 2 // Double the delay for next retry
			continue
		}

		// Check for 429 (Throttling) or 5xx (Server Error) and retry
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			fmt.Printf("  ...API error (%s), retrying in %v\n", resp.Status, delay)
			resp.Body.Close() // Must close body before retrying
			time.Sleep(delay)
			delay *= 2
			continue
		}

		// Any other status code is either success or a non-retryable error
		break
	}

	// If all retries failed on a network error
	if reqErr != nil {
		return fmt.Errorf("all retries failed: %w", reqErr)
	}
	defer resp.Body.Close()

	// Handle non-OK, non-retryable status codes
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&errResp) == nil {
			return fmt.Errorf("API request failed with status %s: %v", resp.Status, errResp)
		}
		return fmt.Errorf("API request failed with status %s", resp.Status)
	}

	// 5. Parse the successful response
	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return fmt.Errorf("failed to decode API response: %w", err)
	}

	// Extract the caption text
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
