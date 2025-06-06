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
	// Get API key from environment or .env file
	apiKey := getApiKey()
	if apiKey == "" {
		fmt.Println("ERROR: IQAir API key not found. Please set IQAIR_API_KEY environment variable or add it to .env file.")
		os.Exit(1)
	}

	fmt.Printf("Testing IQAir API with key: %s... (length: %d)\n", apiKey[:4], len(apiKey))

	// Test with coordinates (New York)
	testNearestCity(apiKey, 40.7128, -74.0060)

	// Test with city name
	testCity(apiKey, "New York", "New York", "USA")
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

func testNearestCity(apiKey string, lat, lon float64) {
	fmt.Println("\n=== Testing nearest_city endpoint ===")
	url := fmt.Sprintf("https://api.airvisual.com/v2/nearest_city?lat=%.6f&lon=%.6f&key=%s", lat, lon, apiKey)
	
	fmt.Printf("API URL: %s\n", strings.Replace(url, apiKey, "[REDACTED]", 1))
	
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("User-Agent", "IQAirAPITester/1.0")
	
	fmt.Println("Sending request to IQAir API...")
	
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("ERROR: Failed to call IQAir API: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	fmt.Printf("Response status: %d\n", resp.StatusCode)
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read response body: %v\n", err)
		return
	}
	
	// Pretty print the JSON response
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, bodyBytes, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: Failed to format JSON: %v\n", err)
		fmt.Printf("Raw response: %s\n", string(bodyBytes))
	} else {
		fmt.Printf("Response body:\n%s\n", prettyJSON.String())
	}
	
	// Parse the response
	var response IQAirResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		fmt.Printf("ERROR: Failed to parse JSON response: %v\n", err)
		return
	}
	
	// Display key information
	if response.Status == "success" {
		fmt.Printf("\nSuccessfully retrieved data for %s, %s, %s\n", 
			response.Data.City, 
			response.Data.State, 
			response.Data.Country)
		fmt.Printf("Current AQI (US): %d (%s)\n", 
			response.Data.Current.Pollution.Aqius,
			getAQICategory(response.Data.Current.Pollution.Aqius))
		fmt.Printf("Main Pollutant: %s\n", getPollutantName(response.Data.Current.Pollution.Mainus))
		fmt.Printf("Temperature: %.1f°C\n", response.Data.Current.Weather.Tp)
	} else {
		fmt.Printf("API returned status: %s\n", response.Status)
	}
}

func testCity(apiKey string, city, state, country string) {
	fmt.Println("\n=== Testing city endpoint ===")
	url := fmt.Sprintf("https://api.airvisual.com/v2/city?city=%s&state=%s&country=%s&key=%s", 
		strings.ReplaceAll(city, " ", "%20"), 
		strings.ReplaceAll(state, " ", "%20"), 
		strings.ReplaceAll(country, " ", "%20"), 
		apiKey)
	
	fmt.Printf("API URL: %s\n", strings.Replace(url, apiKey, "[REDACTED]", 1))
	
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("User-Agent", "IQAirAPITester/1.0")
	
	fmt.Println("Sending request to IQAir API...")
	
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("ERROR: Failed to call IQAir API: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	fmt.Printf("Response status: %d\n", resp.StatusCode)
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read response body: %v\n", err)
		return
	}
	
	// Pretty print the JSON response
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, bodyBytes, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: Failed to format JSON: %v\n", err)
		fmt.Printf("Raw response: %s\n", string(bodyBytes))
	} else {
		fmt.Printf("Response body:\n%s\n", prettyJSON.String())
	}
	
	// Parse the response
	var response IQAirResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		fmt.Printf("ERROR: Failed to parse JSON response: %v\n", err)
		return
	}
	
	// Display key information
	if response.Status == "success" {
		fmt.Printf("\nSuccessfully retrieved data for %s, %s, %s\n", 
			response.Data.City, 
			response.Data.State, 
			response.Data.Country)
		fmt.Printf("Current AQI (US): %d (%s)\n", 
			response.Data.Current.Pollution.Aqius,
			getAQICategory(response.Data.Current.Pollution.Aqius))
		fmt.Printf("Main Pollutant: %s\n", getPollutantName(response.Data.Current.Pollution.Mainus))
		fmt.Printf("Temperature: %.1f°C\n", response.Data.Current.Weather.Tp)
	} else {
		fmt.Printf("API returned status: %s\n", response.Status)
	}
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