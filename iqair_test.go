package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestIQAirAPI tests the IQAir API directly
func TestIQAirAPI(t *testing.T) {
	// Get API key from environment or .env file
	apiKey := getIQAirAPIKey()
	if apiKey == "" {
		t.Fatal("IQAir API key not found. Please set IQAIR_API_KEY environment variable or add it to .env file.")
	}

	fmt.Printf("Testing IQAir API with key: %s... (length: %d)\n", apiKey[:4], len(apiKey))

	// Test multiple locations to ensure the API is working
	testLocations := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"New York", 40.7128, -74.0060},
		{"London", 51.5074, -0.1278},
		{"Tokyo", 35.6762, 139.6503},
		{"Sydney", -33.8688, 151.2093},
		{"Rio de Janeiro", -22.9068, -43.1729},
	}

	for i, loc := range testLocations {
		fmt.Printf("\n=== TEST #%d: %s (%.4f, %.4f) ===\n", i+1, loc.name, loc.lat, loc.lon)
		
		// Add delay between requests to avoid rate limiting
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		
		// Test nearest_city endpoint
		testNearestCity(t, apiKey, loc.lat, loc.lon)
	}

	// Log the total number of successful tests
	fmt.Printf("\n=== COMPLETED %d TESTS ===\n", len(testLocations))
}

func getIQAirAPIKey() string {
	// First check environment variable
	apiKey := os.Getenv("IQAIR_API_KEY")
	if apiKey != "" {
		return apiKey
	}

	// Try to load from .env file
	envData, err := os.ReadFile(".env")
	if err != nil {
		fmt.Printf("Warning: Could not read .env file: %v\n", err)
		return ""
	}

	lines := strings.Split(string(envData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Invalid format
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "IQAIR_API_KEY" {
			return value
		}
	}

	return ""
}

func testNearestCity(t *testing.T, apiKey string, lat, lon float64) {
	// Add timestamp parameter to prevent caching
	timestamp := time.Now().UnixNano()
	url := fmt.Sprintf("https://api.airvisual.com/v2/nearest_city?lat=%.6f&lon=%.6f&key=%s&_t=%d", 
		lat, lon, apiKey, timestamp)
	
	fmt.Printf("API URL: %s\n", strings.Replace(url, apiKey, "[REDACTED]", 1))
	fmt.Printf("Timestamp: %s\n", time.Now().Format(time.RFC3339))
	
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("User-Agent", "IQAirTester/1.0")
	req.Header.Add("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Add("Pragma", "no-cache")
	
	fmt.Println("Sending request to IQAir API...")
	
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to call IQAir API: %v", err)
		return
	}
	defer resp.Body.Close()
	
	fmt.Printf("Response status: %d\n", resp.StatusCode)
	
	// Log response headers
	fmt.Println("Response headers:")
	for name, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", name, value)
		}
	}
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
		return
	}
	
	// Log the raw response
	responseStr := string(bodyBytes)
	fmt.Printf("Response body length: %d bytes\n", len(responseStr))
	
	// Pretty-print the first 1000 characters of the response if it's too long
	if len(responseStr) > 1000 {
		fmt.Printf("Response body (truncated): %s...\n", responseStr[:1000])
	} else {
		fmt.Printf("Response body: %s\n", responseStr)
	}
	
	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
		return
	}
	
	// Check if the API call was successful
	status, ok := result["status"].(string)
	if !ok {
		t.Fatal("Status field not found in response")
		return
	}
	
	if status != "success" {
		t.Fatalf("API returned status: %s", status)
		return
	}
	
	// Log the successful API call to a file
	logFile, err := os.OpenFile("iqair_api_calls.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer logFile.Close()
		timestamp := time.Now().Format(time.RFC3339)
		logFile.WriteString(fmt.Sprintf("[%s] IQAir API call successful: lat=%.6f, lon=%.6f\n", 
			timestamp, lat, lon))
	}
	
	fmt.Println("Successfully received data from IQAir API")
}

// Run this test with:
// go test -v -run TestIQAirAPI