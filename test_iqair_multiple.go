package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// IQAirResponse represents the structure of the IQAir API response
type IQAirResponse struct {
	Status string `json:"status"`
	Data   struct {
		City     string `json:"city"`
		State    string `json:"state"`
		Country  string `json:"country"`
		Location struct {
			Type        string    `json:"type"`
			Coordinates []float64 `json:"coordinates"`
		} `json:"location"`
		Current struct {
			Weather struct {
				Ts string  `json:"ts"`
				Tp float64 `json:"tp"`
				Pr float64 `json:"pr"`
				Hu float64 `json:"hu"`
				Ws float64 `json:"ws"`
				Wd float64 `json:"wd"`
				Ic string  `json:"ic"`
			} `json:"weather"`
			Pollution struct {
				Ts     string  `json:"ts"`
				Aqius  int     `json:"aqius"`  // AQI value (US)
				Mainus string  `json:"mainus"` // Main pollutant (US)
				Aqicn  int     `json:"aqicn"`  // AQI value (China)
				Maincn string  `json:"maincn"` // Main pollutant (China)
				P2     float64 `json:"p2"`     // PM2.5
				P1     float64 `json:"p1"`     // PM10
				O3     float64 `json:"o3"`     // Ozone
				N2     float64 `json:"n2"`     // Nitrogen dioxide
				S2     float64 `json:"s2"`     // Sulfur dioxide
				Co     float64 `json:"co"`     // Carbon monoxide
			} `json:"pollution"`
		} `json:"current"`
	} `json:"data"`
}

func main() {
	// Open a log file
	logFile, err := os.OpenFile("iqair_multiple_tests.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("ERROR: Could not open log file: %v\n", err)
	} else {
		defer logFile.Close()
		logFile.WriteString(fmt.Sprintf("=== IQAir API Test Started at %s ===\n", time.Now().Format(time.RFC3339)))
	}

	// Get API key from environment or .env file
	apiKey := getApiKey()
	if apiKey == "" {
		fmt.Println("ERROR: IQAir API key not found. Please set IQAIR_API_KEY environment variable or add it to .env file.")
		os.Exit(1)
	}

	fmt.Printf("Testing IQAir API with key: %s... (length: %d)\n", apiKey[:4], len(apiKey))
	logMessage(logFile, fmt.Sprintf("Using API key starting with: %s... (length: %d)", apiKey[:4], len(apiKey)))

	// Test locations
	locations := []struct {
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

	successCount := 0

	// Test each location
	for i, loc := range locations {
		fmt.Printf("\n=== TEST #%d: %s (%.4f, %.4f) ===\n", i+1, loc.name, loc.lat, loc.lon)
		logMessage(logFile, fmt.Sprintf("=== TEST #%d: %s (%.4f, %.4f) ===", i+1, loc.name, loc.lat, loc.lon))
		
		// Add a delay between requests to avoid rate limiting
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		
		// Test the nearest_city endpoint
		if testNearestCity(apiKey, loc.lat, loc.lon, logFile) {
			successCount++
		}
	}

	// Log summary
	summary := fmt.Sprintf("\n=== TEST SUMMARY: %d/%d successful API calls ===\n", 
		successCount, len(locations))
	fmt.Print(summary)
	logMessage(logFile, summary)
}

func getApiKey() string {
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

func testNearestCity(apiKey string, lat, lon float64, logFile *os.File) bool {
	// Add timestamp to prevent caching
	timestamp := time.Now().UnixNano()
	url := fmt.Sprintf("https://api.airvisual.com/v2/nearest_city?lat=%.6f&lon=%.6f&key=%s&_t=%d", 
		lat, lon, apiKey, timestamp)
	
	logURLStr := fmt.Sprintf("API URL: %s", strings.Replace(url, apiKey, "[REDACTED]", 1))
	fmt.Println(logURLStr)
	logMessage(logFile, logURLStr)
	
	timeStr := fmt.Sprintf("Request time: %s", time.Now().Format(time.RFC3339))
	fmt.Println(timeStr)
	logMessage(logFile, timeStr)
	
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("User-Agent", "IQAirAPITester/1.0")
	req.Header.Add("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Add("Pragma", "no-cache")
	
	fmt.Println("Sending request to IQAir API...")
	logMessage(logFile, "Sending request to IQAir API...")
	
	resp, err := client.Do(req)
	if err != nil {
		errMsg := fmt.Sprintf("ERROR: Failed to call IQAir API: %v", err)
		fmt.Println(errMsg)
		logMessage(logFile, errMsg)
		return false
	}
	defer resp.Body.Close()
	
	statusMsg := fmt.Sprintf("Response status: %d", resp.StatusCode)
	fmt.Println(statusMsg)
	logMessage(logFile, statusMsg)
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		errMsg := fmt.Sprintf("ERROR: Failed to read response body: %v", err)
		fmt.Println(errMsg)
		logMessage(logFile, errMsg)
		return false
	}
	
	// Pretty print the JSON response
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, bodyBytes, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: Failed to format JSON: %v\n", err)
		fmt.Printf("Raw response: %s\n", string(bodyBytes))
		logMessage(logFile, fmt.Sprintf("Raw response: %s", string(bodyBytes)))
	} else {
		fmt.Printf("Response body:\n%s\n", prettyJSON.String())
		logMessage(logFile, fmt.Sprintf("Response body (formatted):\n%s", prettyJSON.String()))
	}
	
	// Parse the response
	var response IQAirResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		errMsg := fmt.Sprintf("ERROR: Failed to parse JSON response: %v", err)
		fmt.Println(errMsg)
		logMessage(logFile, errMsg)
		return false
	}
	
	// Check if the call was successful
	if response.Status != "success" {
		errMsg := fmt.Sprintf("API returned status: %s", response.Status)
		fmt.Println(errMsg)
		logMessage(logFile, errMsg)
		return false
	}
	
	// Display key information
	successMsg := fmt.Sprintf("\nSuccessfully retrieved data for %s, %s, %s", 
		response.Data.City, 
		response.Data.State, 
		response.Data.Country)
	fmt.Println(successMsg)
	logMessage(logFile, successMsg)
	
	aqiMsg := fmt.Sprintf("Current AQI (US): %d (%s)", 
		response.Data.Current.Pollution.Aqius,
		getAQICategory(response.Data.Current.Pollution.Aqius))
	fmt.Println(aqiMsg)
	logMessage(logFile, aqiMsg)
	
	pollutantMsg := fmt.Sprintf("Main Pollutant: %s", 
		getPollutantName(response.Data.Current.Pollution.Mainus))
	fmt.Println(pollutantMsg)
	logMessage(logFile, pollutantMsg)
	
	tempMsg := fmt.Sprintf("Temperature: %.1f°C", response.Data.Current.Weather.Tp)
	fmt.Println(tempMsg)
	logMessage(logFile, tempMsg)
	
	// Log the successful API call
	successLog := fmt.Sprintf("[%s] IQAir API call SUCCESS: lat=%.6f, lon=%.6f, city=%s, AQI=%d", 
		time.Now().Format(time.RFC3339), 
		lat, lon, 
		response.Data.City,
		response.Data.Current.Pollution.Aqius)
	logMessage(logFile, successLog)
	
	return true
}

func getAQICategory(aqi int) string {
	switch {
	case aqi <= 50:
		return "Good"
	case aqi <= 100:
		return "Moderate"
	case aqi <= 150:
		return "Unhealthy for Sensitive Groups"
	case aqi <= 200:
		return "Unhealthy"
	case aqi <= 300:
		return "Very Unhealthy"
	default:
		return "Hazardous"
	}
}

func getPollutantName(pollutant string) string {
	switch pollutant {
	case "p2":
		return "PM2.5"
	case "p1":
		return "PM10"
	case "o3":
		return "Ozone"
	case "n2":
		return "Nitrogen Dioxide"
	case "s2":
		return "Sulfur Dioxide"
	case "co":
		return "Carbon Monoxide"
	default:
		return pollutant
	}
}

func logMessage(logFile *os.File, message string) {
	if logFile != nil {
		logFile.WriteString(message + "\n")
	}
}