package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sagan/goaider/cmd"
	"github.com/sagan/goaider/constants"
)

// --- Constants for API and Retry Logic ---
const (
	// Set base backoff to 6s to respect the default 10 RPM quota
	baseBackoff = 6 * time.Second
	maxBackoff  = 60 * time.Second
	maxRetries  = 4 // 4 retries = 5 total attempts
)

var (
	flagDir   string
	flagForce bool
	flagModel string
)

// sttCmd represents the stt command
var sttCmd = &cobra.Command{
	Use:   "stt",
	Short: "Generates speech-to-text transcripts for audio files",
	Long: `Processes a directory of audio files (.wav, .mp3, .m4a, .flac, .ogg)
and generates a corresponding .txt file for each one using the
Google Gemini API.

Implements exponential backoff to handle rate limiting (e.g., 10 RPM).

Requires the GEMINI_API_KEY environment variable to be set.`,
	// This is the main function that runs when the command is called
	RunE: stt,
}

func init() {
	cmd.RootCmd.AddCommand(sttCmd)
	sttCmd.Flags().StringVarP(&flagDir, "dir", "", "", "Directory containing audio files (required)")
	sttCmd.Flags().BoolVarP(&flagForce, "force", "", false, "Overwrite existing .txt transcript files")
	sttCmd.Flags().StringVarP(&flagModel, "model", "", constants.DEFAULT_GEMINI_MODEL, "The model to use for transcription")
	sttCmd.MarkFlagRequired("dir")
}

func stt(cmd *cobra.Command, args []string) error {
	apiKey := os.Getenv(constants.ENV_GEMINI_API_KEY)
	if apiKey == "" {
		return fmt.Errorf("error: %s environment variable not set", constants.ENV_GEMINI_API_KEY)
	}

	fmt.Printf("Processing audio files in: %q\n", flagDir)
	fmt.Printf("Using model: %s\n", flagModel)

	// Read all files in the directory
	files, err := os.ReadDir(flagDir)
	if err != nil {
		return fmt.Errorf("error reading directory %q: %w", flagDir, err)
	}

	// 60-second timeout for a single request, but retries can make this longer.
	httpClient := &http.Client{Timeout: 60 * time.Second}

	errorCnt := 0
	for _, file := range files {
		if file.IsDir() {
			continue // Skip subdirectories
		}

		fileName := file.Name()
		fileExt := strings.ToLower(filepath.Ext(fileName))
		mimeType := getMimeType(fileExt)

		if mimeType == "" {
			// fmt.Printf("Skipping non-audio file: %s\n", fileName)
			continue // Not a supported audio file
		}

		// Define input and output paths
		audioFilePath := filepath.Join(flagDir, fileName)
		outputTxtPath := strings.TrimSuffix(audioFilePath, fileExt) + ".txt"

		// Check if output file exists
		if !flagForce {
			if _, err := os.Stat(outputTxtPath); err == nil {
				fmt.Printf("Skipping (exists): %s\n", fileName)
				continue
			}
		}

		// Process the file
		fmt.Printf("Processing: %s\n", fileName)

		// 1. Read audio file
		audioData, err := os.ReadFile(audioFilePath)
		if err != nil {
			log.Printf("Error reading audio file %s: %v", fileName, err)
			errorCnt++
			continue
		}

		// 2. Call Gemini API
		transcript, err := getTranscript(httpClient, apiKey, flagModel, audioData, mimeType)
		if err != nil {
			log.Printf("Error generating transcript for %s: %v", fileName, err)
			errorCnt++
			continue
		}

		// 3. Write transcript to .txt file
		err = os.WriteFile(outputTxtPath, []byte(transcript), 0644)
		if err != nil {
			log.Printf("Error writing transcript file %s: %v", outputTxtPath, err)
			errorCnt++
			continue
		}

		fmt.Printf("Generated: %s\n", filepath.Base(outputTxtPath))
	}

	fmt.Printf("Processing complete.\n")
	if errorCnt > 0 {
		return fmt.Errorf("%d errors", errorCnt)
	}
	return nil
}

// Structs for Gemini API Request
type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // Base64 encoded string
}

// Structs for Gemini API Response
type GeminiResponse struct {
	Candidates     []Candidate     `json:"candidates"`
	PromptFeedback *PromptFeedback `json:"promptFeedback,omitempty"`
}

type Candidate struct {
	Content       Content        `json:"content"`
	FinishReason  string         `json:"finishReason"`
	Index         int            `json:"index"`
	SafetyRatings []SafetyRating `json:"safetyRatings"`
}

type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type PromptFeedback struct {
	BlockReason   string         `json:"blockReason,omitempty"`
	SafetyRatings []SafetyRating `json:"safetyRatings,omitempty"`
}

// getTranscript calls the Gemini API with retry logic
func getTranscript(client *http.Client, apiKey, modelName string, audioData []byte, mimeType string) (string, error) {
	// 1. Base64 encode the audio
	encodedData := base64.StdEncoding.EncodeToString(audioData)

	// 2. Prepare the request body
	reqBody := GeminiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: "Generate a transcript of this audio. Only output the transcribed text."},
					{InlineData: &InlineData{
						MimeType: mimeType,
						Data:     encodedData,
					}},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON request: %w", err)
	}

	// 3. Build the URL
	url := fmt.Sprintf("%s%s:generateContent?key=%s", constants.GEMINI_API_URL, modelName, apiKey)

	var lastErr error

	// 4. Start retry loop
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Create a new request *inside* the loop because the body buffer must be fresh
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", fmt.Errorf("failed to create HTTP request: %w", err) // Non-retryable
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			// Network error
			lastErr = fmt.Errorf("request failed: %w", err)
			log.Printf("Attempt %d/%d: Network error (%v). Retrying...", attempt+1, maxRetries+1, err)
			time.Sleep(calculateBackoff(attempt))
			continue
		}

		// Check status code
		switch resp.StatusCode {
		case http.StatusOK:
			// Success!
			respBody, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return "", fmt.Errorf("failed to read successful API response body: %w", err)
			}

			// Parse the response
			var apiResp GeminiResponse
			if err := json.Unmarshal(respBody, &apiResp); err != nil {
				return "", fmt.Errorf("failed to unmarshal API response: %w", err)
			}

			// Check for blocked content
			if apiResp.PromptFeedback != nil && apiResp.PromptFeedback.BlockReason != "" {
				return "", fmt.Errorf("request was blocked: %s", apiResp.PromptFeedback.BlockReason)
			}

			// Extract the text
			if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
				return "", fmt.Errorf("no transcript content found in API response: %s", string(respBody))
			}
			transcript := apiResp.Candidates[0].Content.Parts[0].Text
			return transcript, nil // SUCCESS EXIT

		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			// Retryable server-side error (429 or 5xx)
			respBody, _ := io.ReadAll(resp.Body) // Read body for logging, ignore error
			resp.Body.Close()
			lastErr = fmt.Errorf("API returned retryable status %d: %s", resp.StatusCode, string(respBody))
			log.Printf("Attempt %d/%d: %v. Retrying in %v...", attempt+1, maxRetries+1, lastErr, calculateBackoff(attempt))
			time.Sleep(calculateBackoff(attempt))
			continue

		default:
			// Non-retryable client-side error (e.g., 400, 401, 404)
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("API request failed with non-retryable status %d: %s", resp.StatusCode, string(respBody))
		}
	} // end for loop

	// If loop finishes, all retries failed
	return "", fmt.Errorf("all %d retry attempts failed. Last error: %w", maxRetries+1, lastErr)
}

// calculateBackoff computes the exponential backoff duration for a given attempt
func calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: base * 2^attempt
	backoff := baseBackoff * (1 << attempt)
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	// Add random jitter (0-1000ms) to prevent thundering herd
	jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
	return backoff + jitter
}

// --- Helpers ---

// getMimeType maps file extensions to their MIME types for the API
func getMimeType(ext string) string {
	switch ext {
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/m4a"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	default:
		return "" // Not a supported type
	}
}
