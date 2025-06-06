package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// IQAir API key environment variable name
const IQAIR_API_KEY_ENV = "IQAIR_API_KEY"

// Configuration
type Config struct {
	WeatherAPIKey  string
	LLMAPIKey      string
	IQAirAPIKey    string
	City           string
	CountryCode    string
	CheckInterval  int
	Units          string
	LogToFile      bool
	LogFile        string
	LLMProvider    string // "anthropic", "openai", etc.
	LLMModel       string // "claude-3-5-sonnet", "gpt-4", etc.
	LLMTemperature float64
	SystemPrompt   string
}

// Weather data from OpenWeatherMap API
// Update the WeatherResponse struct to include isDay field
type WeatherResponse struct {
	Weather []struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
		TempMin   float64 `json:"temp_min"`
		TempMax   float64 `json:"temp_max"`
		Pressure  int     `json:"pressure"`
		Humidity  int     `json:"humidity"`
	} `json:"main"`
	Wind struct {
		Speed float64 `json:"speed"`
		Deg   int     `json:"deg"`
		Gust  float64 `json:"gust"`
	} `json:"wind"`
	Clouds struct {
		All int `json:"all"`
	} `json:"clouds"`
	Rain struct {
		OneHour    float64 `json:"1h,omitempty"`
		ThreeHours float64 `json:"3h,omitempty"`
	} `json:"rain,omitempty"`
	Snow struct {
		OneHour    float64 `json:"1h,omitempty"`
		ThreeHours float64 `json:"3h,omitempty"`
	} `json:"snow,omitempty"`
	Visibility int    `json:"visibility"`
	Name       string `json:"name"`
	Sys        struct {
		Country string `json:"country"`
		Sunrise int64  `json:"sunrise"`
		Sunset  int64  `json:"sunset"`
	} `json:"sys"`
	Timezone int   `json:"timezone"` // Timezone offset in seconds
	Dt       int64 `json:"dt"`       // Time of data calculation, unix
	IsDay    int   `json:"is_day"`   // 1 for day, 0 for night
	AQI struct {
		List []struct {
			Main struct {
				AQI int `json:"aqi"` // Air Quality Index
			} `json:"main"`
			Components struct {
				CO    float64 `json:"co"`    // Carbon monoxide (μg/m3)
				NO    float64 `json:"no"`    // Nitrogen monoxide (μg/m3)
				NO2   float64 `json:"no2"`   // Nitrogen dioxide (μg/m3)
				O3    float64 `json:"o3"`    // Ozone (μg/m3)
				SO2   float64 `json:"so2"`   // Sulphur dioxide (μg/m3)
				PM2_5 float64 `json:"pm2_5"` // Fine particles (μg/m3)
				PM10  float64 `json:"pm10"`  // Coarse particles (μg/m3)
				NH3   float64 `json:"nh3"`   // Ammonia (μg/m3)
			} `json:"components"`
		} `json:"list"`
	} `json:"aqi,omitempty"` // Air Quality data
	
	// Additional AQI data from IQAir
	IQAirData struct {
		AQI            int     `json:"aqi"`            // US AQI
		Category       string  `json:"category"`       // Category (Good, Moderate, etc.)
		PollutantName  string  `json:"pollutant_name"` // Main pollutant
		PollutantValue float64 `json:"pollutant_value"`
		PollutantUnit  string  `json:"pollutant_unit"`
		PM25           float64 `json:"pm25"` // PM2.5 concentration
		PM10           float64 `json:"pm10"` // PM10 concentration
	} `json:"iqair_data,omitempty"`
}

// Anthropic API structures
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
	MaxTokens   int                `json:"max_tokens"`
}

type AnthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
}

// OpenAI API structures
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

// WeatherAgent structure
type WeatherAgent struct {
	config          Config
	logger          *log.Logger
	weatherHistory  []WeatherResponse
	lastMessageTime time.Time
	lastMessage     string
}

// Initialize a new WeatherAgent
func NewWeatherAgent(config Config) *WeatherAgent {
	// Set up logging
	var logger *log.Logger
	if config.LogToFile {
		file, err := os.OpenFile(config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Error opening log file: %v, using standard logging", err)
			logger = log.New(os.Stdout, "", log.LstdFlags)
		} else {
			logger = log.New(io.MultiWriter(os.Stdout, file), "", log.LstdFlags)
		}
	} else {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	// Default system prompt if none provided
	if config.SystemPrompt == "" {
		config.SystemPrompt = `You are a helpful AI weather assistant. Your task is to analyze weather data and provide helpful, engaging, and contextual messages about the current weather.

Some guidelines:
1. Be conversational and personable
2. Vary your messages to avoid repetition
3. Include practical advice based on the weather conditions
4. Note significant changes in weather when they occur
5. Mention the time of day and how it relates to the weather when relevant
6. Make appropriate seasonal references
7. Keep responses concise and focused (1-3 sentences)
8. Occasionally include interesting weather facts
9. Adjust your tone based on severe weather (more serious for dangerous conditions)

Your messages should be directly useful to someone wondering about current weather conditions.`
	}

	agent := &WeatherAgent{
		config:          config,
		logger:          logger,
		weatherHistory:  make([]WeatherResponse, 0, 24), // Store up to 24 hours of history
		lastMessageTime: time.Time{},
	}

	return agent
}

// Add this geocoding function to your code
// Get coordinates for a city name using Open-Meteo Geocoding API
func (agent *WeatherAgent) getCoordinates(city, country string) (float64, float64, error) {
	// URL encode the city and country
	cityEncoded := url.QueryEscape(city)

	// Use the Open-Meteo Geocoding API
	geocodeURL := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1", cityEncoded)

	// Add country code if provided
	if country != "" {
		geocodeURL += fmt.Sprintf("&country=%s", strings.ToLower(country))
	}

	resp, err := http.Get(geocodeURL)
	if err != nil {
		return 0, 0, fmt.Errorf("geocoding request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return 0, 0, fmt.Errorf("geocoding API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse the geocoding response
	var geocodeResp struct {
		Results []struct {
			Name        string  `json:"name"`
			Country     string  `json:"country"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			CountryCode string  `json:"country_code"`
		} `json:"results"`
	}

	err = json.NewDecoder(resp.Body).Decode(&geocodeResp)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse geocoding response: %v", err)
	}

	// Check if we got any results
	if len(geocodeResp.Results) == 0 {
		return 0, 0, fmt.Errorf("no locations found for %s, %s", city, country)
	}

	// Use the first result
	result := geocodeResp.Results[0]

	// Log the resolved location
	agent.logger.Printf("Resolved location: %s, %s (%.4f, %.4f)",
		result.Name, result.Country, result.Latitude, result.Longitude)

	return result.Latitude, result.Longitude, nil
}

// Fetch current weather from OpenWeatherMap API
// Fetch current weather from Open-Meteo API
// Now modify the fetchWeather function to use geocoding
// Modify the fetchWeather function to request timezone information
func (agent *WeatherAgent) fetchWeather() (WeatherResponse, error) {
	// Get coordinates for the city
	lat, lon, err := agent.getCoordinates(agent.config.City, agent.config.CountryCode)
	if err != nil {
		// Fall back to default coordinates if geocoding fails
		agent.logger.Printf("Geocoding failed: %v. Using default coordinates for London.", err)
		lat, lon = 51.5074, -0.1278 // Default to London
	}

	// Get the temperature_unit parameter based on config
	tempUnit := "celsius"
	windUnit := "kmh"
	if agent.config.Units == "imperial" {
		tempUnit = "fahrenheit"
		windUnit = "mph"
	}

	// Add temperature_unit, windspeed_unit, and timezone parameters to the URL
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,cloud_cover,wind_speed_10m,wind_direction_10m,is_day&temperature_unit=%s&windspeed_unit=%s&timezone=auto",
		lat, lon, tempUnit, windUnit)

	resp, err := http.Get(url)
	if err != nil {
		return WeatherResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return WeatherResponse{}, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse Open-Meteo response with timezone information
	var openMeteoResp struct {
		Current struct {
			Temperature      float64 `json:"temperature_2m"`
			ApparentTemp     float64 `json:"apparent_temperature"`
			RelativeHumidity int     `json:"relative_humidity_2m"`
			Precipitation    float64 `json:"precipitation"`
			WeatherCode      int     `json:"weather_code"`
			CloudCover       int     `json:"cloud_cover"`
			WindSpeed        float64 `json:"wind_speed_10m"`
			WindDirection    int     `json:"wind_direction_10m"`
			Time             string  `json:"time"`
			IsDay            int     `json:"is_day"` // 1 for day, 0 for night
		} `json:"current"`
		CurrentUnits struct {
			Temperature string `json:"temperature_2m"`
			WindSpeed   string `json:"wind_speed_10m"`
		} `json:"current_units"`
		Timezone       string `json:"timezone"`
		TimezoneAbbr   string `json:"timezone_abbreviation"`
		TimezoneOffset int    `json:"utc_offset_seconds"`
	}

	err = json.NewDecoder(resp.Body).Decode(&openMeteoResp)
	if err != nil {
		return WeatherResponse{}, err
	}

	// Parse the local time from the API response and apply timezone
	// Open-Meteo returns time in local timezone, but Go parses it as if it's in server timezone
	// We need to create the proper timezone location first

	// Create a fixed offset timezone based on the API's timezone offset
	locationTimezone := time.FixedZone("Local", openMeteoResp.TimezoneOffset)

	var localTime time.Time
	timeFormats := []string{
		time.RFC3339,       // Try full RFC3339 format first: "2006-01-02T15:04:05Z07:00"
		"2006-01-02T15:04", // Try format without seconds: "2006-01-02T15:04"
		"2006-01-02 15:04", // Try alternative format
	}

	var parseErr error
	for _, format := range timeFormats {
		// Parse the time and interpret it as being in the location's timezone
		parsedTime, err := time.Parse(format, openMeteoResp.Current.Time)
		if err == nil {
			// Convert the parsed time to be in the correct timezone
			localTime = time.Date(
				parsedTime.Year(), parsedTime.Month(), parsedTime.Day(),
				parsedTime.Hour(), parsedTime.Minute(), parsedTime.Second(),
				parsedTime.Nanosecond(), locationTimezone)
			parseErr = nil
			break
		}
		parseErr = err
	}

	if parseErr != nil {
		// Fall back to current time if parsing fails
		localTime = time.Now().In(locationTimezone)
		agent.logger.Printf("Failed to parse time from API: %v (time string: '%s'). Using current time.",
			parseErr, openMeteoResp.Current.Time)
	}

	// Debug the time format received from the API
	agent.logger.Printf("API returned time string: '%s'", openMeteoResp.Current.Time)

	// Convert to our standard WeatherResponse format
	weather := WeatherResponse{
		Weather: []struct {
			ID          int    `json:"id"`
			Main        string `json:"main"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		}{
			{
				ID:          openMeteoResp.Current.WeatherCode,
				Main:        agent.weatherCodeToCondition(openMeteoResp.Current.WeatherCode),
				Description: agent.weatherCodeToDescription(openMeteoResp.Current.WeatherCode),
				Icon:        "",
			},
		},
		Main: struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			TempMin   float64 `json:"temp_min"`
			TempMax   float64 `json:"temp_max"`
			Pressure  int     `json:"pressure"`
			Humidity  int     `json:"humidity"`
		}{
			Temp:      openMeteoResp.Current.Temperature,
			FeelsLike: openMeteoResp.Current.ApparentTemp,
			Humidity:  openMeteoResp.Current.RelativeHumidity,
		},
		Wind: struct {
			Speed float64 `json:"speed"`
			Deg   int     `json:"deg"`
			Gust  float64 `json:"gust"`
		}{
			Speed: openMeteoResp.Current.WindSpeed,
			Deg:   openMeteoResp.Current.WindDirection,
		},
		Clouds: struct {
			All int `json:"all"`
		}{
			All: openMeteoResp.Current.CloudCover,
		},
		Name: agent.config.City,
		Sys: struct {
			Country string `json:"country"`
			Sunrise int64  `json:"sunrise"`
			Sunset  int64  `json:"sunset"`
		}{
			Country: agent.config.CountryCode,
		},
		Dt:       localTime.Unix(),             // Time in correct timezone
		Timezone: openMeteoResp.TimezoneOffset, // Store timezone offset for reference
	}

	// Debug timezone information
	agent.logger.Printf("Location timezone: %s (%s), offset: %d seconds",
		openMeteoResp.Timezone, openMeteoResp.TimezoneAbbr, openMeteoResp.TimezoneOffset)
	agent.logger.Printf("Local time at location: %s (is_day: %d)",
		localTime.Format(time.RFC3339), openMeteoResp.Current.IsDay)

	// Generate approximate sunrise/sunset times if they're not available
	// Most weather conditions are day/night dependent
	if weather.Sys.Sunrise == 0 || weather.Sys.Sunset == 0 {
		// Use the isDay flag to provide context about day/night
		if openMeteoResp.Current.IsDay == 1 {
			agent.logger.Printf("It is currently daytime at the location")
		} else {
			agent.logger.Printf("It is currently nighttime at the location")
		}
	}

	// Try to fetch AQI data from IQAir if we have an API key
	if agent.config.IQAirAPIKey != "" {
		fmt.Printf("\n==== INITIATING IQAIR API CALL ====\n")
		fmt.Printf("DEBUG: Using IQAir API key: %s..., length: %d\n", agent.config.IQAirAPIKey[:4], len(agent.config.IQAirAPIKey))
		fmt.Printf("DEBUG: Coordinates: lat=%.6f, lon=%.6f\n", lat, lon)
		agent.logger.Printf("DEBUG: Using IQAir API key: %s..., length: %d", agent.config.IQAirAPIKey[:4], len(agent.config.IQAirAPIKey))
		
		// Force a fresh call to the IQAir API
		agent.fetchIQAirData(&weather, lat, lon)
		
		// Check if IQAir data was successfully added
		if weather.IQAirData.AQI > 0 {
			fmt.Printf("DEBUG: Successfully added IQAir data: AQI=%d, Category=%s\n", 
				weather.IQAirData.AQI, weather.IQAirData.Category)
		} else {
			fmt.Printf("WARNING: IQAir data was not added to the weather response\n")
		}
	} else {
		// Fallback to OpenWeatherMap AQI data
		agent.logger.Printf("No IQAir API key configured, falling back to OpenWeatherMap AQI data")
		
		// Now fetch Air Quality data if coordinates are available
		aqiURL := fmt.Sprintf("https://api.openweathermap.org/data/2.5/air_pollution?lat=%f&lon=%f&appid=%s",
			lat, lon, agent.config.WeatherAPIKey)
		
		agent.logger.Printf("DEBUG: Fetching AQI data from URL: %s", aqiURL)
		
		aqiResp, err := http.Get(aqiURL)
		if err != nil {
			agent.logger.Printf("Warning: Failed to fetch AQI data: %v", err)
			// Continue without AQI data, don't return an error
		} else {
			defer aqiResp.Body.Close()
			agent.logger.Printf("DEBUG: AQI API response status: %d", aqiResp.StatusCode)
			
			if aqiResp.StatusCode == http.StatusOK {
				var aqiData struct {
					List []struct {
						Main struct {
							AQI int `json:"aqi"`
						} `json:"main"`
						Components struct {
							CO    float64 `json:"co"`
							NO    float64 `json:"no"`
							NO2   float64 `json:"no2"`
							O3    float64 `json:"o3"`
							SO2   float64 `json:"so2"`
							PM2_5 float64 `json:"pm2_5"`
							PM10  float64 `json:"pm10"`
							NH3   float64 `json:"nh3"`
						} `json:"components"`
					} `json:"list"`
				}
				
				// Read the response body for logging
				bodyBytes, _ := io.ReadAll(aqiResp.Body)
				agent.logger.Printf("DEBUG: AQI API response body: %s", string(bodyBytes))
				
				// Create a new reader with the same data for decoding
				bodyReader := bytes.NewReader(bodyBytes)
				
				if err := json.NewDecoder(bodyReader).Decode(&aqiData); err != nil {
					agent.logger.Printf("Warning: Failed to decode AQI data: %v", err)
				} else {
					agent.logger.Printf("DEBUG: Decoded AQI data: %+v", aqiData)
					if len(aqiData.List) > 0 {
						// Add AQI data to the weather response
						weather.AQI.List = aqiData.List
						agent.logger.Printf("Successfully added AQI data: %+v", aqiData.List[0])
					} else {
						agent.logger.Printf("Warning: AQI data list is empty")
					}
				}
			} else {
				agent.logger.Printf("Warning: AQI API returned status %d", aqiResp.StatusCode)
			}
		}
	}

	return weather, nil
}

// Fetch weather data using coordinates directly (for geolocation)
func (agent *WeatherAgent) fetchWeatherByCoordinates(lat, lon float64) (WeatherResponse, error) {
	// Get the temperature_unit parameter based on config
	tempUnit := "celsius"
	windUnit := "kmh"
	if agent.config.Units == "imperial" {
		tempUnit = "fahrenheit"
		windUnit = "mph"
	}

	// Use Open-Meteo API with coordinates directly
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,cloud_cover,wind_speed_10m,wind_direction_10m,is_day&temperature_unit=%s&windspeed_unit=%s&timezone=auto",
		lat, lon, tempUnit, windUnit)

	resp, err := http.Get(url)
	if err != nil {
		return WeatherResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return WeatherResponse{}, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse Open-Meteo response with timezone information
	var openMeteoResp struct {
		Current struct {
			Temperature      float64 `json:"temperature_2m"`
			ApparentTemp     float64 `json:"apparent_temperature"`
			RelativeHumidity int     `json:"relative_humidity_2m"`
			Precipitation    float64 `json:"precipitation"`
			WeatherCode      int     `json:"weather_code"`
			CloudCover       int     `json:"cloud_cover"`
			WindSpeed        float64 `json:"wind_speed_10m"`
			WindDirection    int     `json:"wind_direction_10m"`
			Time             string  `json:"time"`
			IsDay            int     `json:"is_day"` // 1 for day, 0 for night
		} `json:"current"`
		CurrentUnits struct {
			Temperature string `json:"temperature_2m"`
			WindSpeed   string `json:"wind_speed_10m"`
		} `json:"current_units"`
		Timezone       string `json:"timezone"`
		TimezoneAbbr   string `json:"timezone_abbreviation"`
		TimezoneOffset int    `json:"utc_offset_seconds"`
	}

	err = json.NewDecoder(resp.Body).Decode(&openMeteoResp)
	if err != nil {
		return WeatherResponse{}, err
	}

	// Parse the local time from the API response and apply timezone
	// Open-Meteo returns time in local timezone, but Go parses it as if it's in server timezone
	// We need to create the proper timezone location first

	// Create a fixed offset timezone based on the API's timezone offset
	locationTimezone := time.FixedZone("Local", openMeteoResp.TimezoneOffset)

	var localTime time.Time
	timeFormats := []string{
		time.RFC3339,       // Try full RFC3339 format first: "2006-01-02T15:04:05Z07:00"
		"2006-01-02T15:04", // Try format without seconds: "2006-01-02T15:04"
		"2006-01-02 15:04", // Try alternative format
	}

	var parseErr error
	for _, format := range timeFormats {
		// Parse the time and interpret it as being in the location's timezone
		parsedTime, err := time.Parse(format, openMeteoResp.Current.Time)
		if err == nil {
			// Convert the parsed time to be in the correct timezone
			localTime = time.Date(
				parsedTime.Year(), parsedTime.Month(), parsedTime.Day(),
				parsedTime.Hour(), parsedTime.Minute(), parsedTime.Second(),
				parsedTime.Nanosecond(), locationTimezone)
			parseErr = nil
			break
		}
		parseErr = err
	}

	if parseErr != nil {
		// Fall back to current time if parsing fails
		localTime = time.Now().In(locationTimezone)
		agent.logger.Printf("Failed to parse time from API: %v (time string: '%s'). Using current time.",
			parseErr, openMeteoResp.Current.Time)
	}

	// Try to get city name from coordinates using reverse geocoding
	cityName, countryCode := agent.reverseGeocode(lat, lon)

	// Convert to our standard WeatherResponse format
	weather := WeatherResponse{
		Weather: []struct {
			ID          int    `json:"id"`
			Main        string `json:"main"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		}{
			{
				ID:          openMeteoResp.Current.WeatherCode,
				Main:        agent.weatherCodeToCondition(openMeteoResp.Current.WeatherCode),
				Description: agent.weatherCodeToDescription(openMeteoResp.Current.WeatherCode),
				Icon:        "",
			},
		},
		Main: struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			TempMin   float64 `json:"temp_min"`
			TempMax   float64 `json:"temp_max"`
			Pressure  int     `json:"pressure"`
			Humidity  int     `json:"humidity"`
		}{
			Temp:      openMeteoResp.Current.Temperature,
			FeelsLike: openMeteoResp.Current.ApparentTemp,
			Humidity:  openMeteoResp.Current.RelativeHumidity,
		},
		Wind: struct {
			Speed float64 `json:"speed"`
			Deg   int     `json:"deg"`
			Gust  float64 `json:"gust"`
		}{
			Speed: openMeteoResp.Current.WindSpeed,
			Deg:   openMeteoResp.Current.WindDirection,
		},
		Clouds: struct {
			All int `json:"all"`
		}{
			All: openMeteoResp.Current.CloudCover,
		},
		Name: cityName,
		Sys: struct {
			Country string `json:"country"`
			Sunrise int64  `json:"sunrise"`
			Sunset  int64  `json:"sunset"`
		}{
			Country: countryCode,
		},
		Dt:       localTime.Unix(),             // Time in correct timezone
		Timezone: openMeteoResp.TimezoneOffset, // Store timezone offset for reference
	}

	// Debug timezone information
	agent.logger.Printf("Location timezone: %s (%s), offset: %d seconds",
		openMeteoResp.Timezone, openMeteoResp.TimezoneAbbr, openMeteoResp.TimezoneOffset)
	agent.logger.Printf("Local time at location: %s (is_day: %d)",
		localTime.Format(time.RFC3339), openMeteoResp.Current.IsDay)

	return weather, nil
}

// Reverse geocode coordinates to get city name with multiple fallbacks
func (agent *WeatherAgent) reverseGeocode(lat, lon float64) (string, string) {
	// Try multiple geocoding services for better reliability

	// Method 1: Try BigDataCloud (no API key required, good for coordinates)
	cityName, countryCode := agent.tryBigDataCloudGeocode(lat, lon)
	if cityName != "" && !strings.Contains(cityName, "Location") {
		return cityName, countryCode
	}

	// Method 2: Try Nominatim as fallback
	cityName, countryCode = agent.tryNominatimGeocode(lat, lon)
	if cityName != "" && !strings.Contains(cityName, "Location") {
		return cityName, countryCode
	}

	// Method 3: Simple coordinate-based city guessing for known areas
	cityName, countryCode = agent.guessLocationFromCoordinates(lat, lon)

	return cityName, countryCode
}

// Try BigDataCloud reverse geocoding (more reliable)
func (agent *WeatherAgent) tryBigDataCloudGeocode(lat, lon float64) (string, string) {
	geocodeURL := fmt.Sprintf("https://api.bigdatacloud.net/data/reverse-geocode-client?latitude=%.6f&longitude=%.6f&localityLanguage=en", lat, lon)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(geocodeURL)
	if err != nil {
		agent.logger.Printf("BigDataCloud geocoding failed: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", ""
	}

	var geocodeResp struct {
		City        string `json:"city"`
		Locality    string `json:"locality"`
		CountryCode string `json:"countryCode"`
		CountryName string `json:"countryName"`
	}

	err = json.NewDecoder(resp.Body).Decode(&geocodeResp)
	if err != nil {
		return "", ""
	}

	cityName := geocodeResp.City
	if cityName == "" {
		cityName = geocodeResp.Locality
	}

	if cityName != "" {
		agent.logger.Printf("BigDataCloud geocoded: %s, %s", cityName, geocodeResp.CountryName)
		return cityName, strings.ToUpper(geocodeResp.CountryCode)
	}

	return "", ""
}

// Try Nominatim with better error handling
func (agent *WeatherAgent) tryNominatimGeocode(lat, lon float64) (string, string) {
	geocodeURL := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?format=json&lat=%.6f&lon=%.6f&zoom=10&addressdetails=1", lat, lon)

	req, err := http.NewRequest("GET", geocodeURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "WeatherAgent/1.0 (+https://github.com/yourname/weather-agent)")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", ""
	}

	var geocodeResp struct {
		Address struct {
			City         string `json:"city"`
			Town         string `json:"town"`
			Village      string `json:"village"`
			Municipality string `json:"municipality"`
			County       string `json:"county"`
			CountryCode  string `json:"country_code"`
		} `json:"address"`
	}

	err = json.NewDecoder(resp.Body).Decode(&geocodeResp)
	if err != nil {
		return "", ""
	}

	cityName := ""
	if geocodeResp.Address.City != "" {
		cityName = geocodeResp.Address.City
	} else if geocodeResp.Address.Town != "" {
		cityName = geocodeResp.Address.Town
	} else if geocodeResp.Address.Village != "" {
		cityName = geocodeResp.Address.Village
	} else if geocodeResp.Address.Municipality != "" {
		cityName = geocodeResp.Address.Municipality
	} else if geocodeResp.Address.County != "" {
		cityName = geocodeResp.Address.County
	}

	if cityName != "" {
		agent.logger.Printf("Nominatim geocoded: %s", cityName)
		return cityName, strings.ToUpper(geocodeResp.Address.CountryCode)
	}

	return "", ""
}

// Fallback: Guess major cities from coordinates (for common locations)
func (agent *WeatherAgent) guessLocationFromCoordinates(lat, lon float64) (string, string) {
	// Common city coordinates (rough approximations)
	cities := []struct {
		name, country string
		lat, lon      float64
		radius        float64 // rough radius in degrees
	}{
		{"New York", "US", 40.7128, -74.0060, 0.5},
		{"Los Angeles", "US", 34.0522, -118.2437, 0.5},
		{"Chicago", "US", 41.8781, -87.6298, 0.3},
		{"London", "GB", 51.5074, -0.1278, 0.3},
		{"Paris", "FR", 48.8566, 2.3522, 0.3},
		{"Tokyo", "JP", 35.6762, 139.6503, 0.5},
		{"Sydney", "AU", -33.8688, 151.2093, 0.3},
		{"Toronto", "CA", 43.6532, -79.3832, 0.3},
	}

	for _, city := range cities {
		// Simple distance calculation
		latDiff := lat - city.lat
		lonDiff := lon - city.lon
		distance := latDiff*latDiff + lonDiff*lonDiff

		if distance < city.radius*city.radius {
			agent.logger.Printf("Guessed location from coordinates: %s, %s", city.name, city.country)
			return city.name, city.country
		}
	}

	// If no match, return coordinates
	return fmt.Sprintf("Location %.2f,%.2f", lat, lon), "Unknown"
}

// Helper function to convert Open-Meteo weather codes to conditions
func (agent *WeatherAgent) weatherCodeToCondition(code int) string {
	// WMO Weather interpretation codes (WW)
	switch {
	case code == 0:
		return "Clear"
	case code == 1:
		return "Mainly Clear"
	case code == 2 || code == 3:
		return "Clouds"
	case code >= 45 && code <= 49:
		return "Fog"
	case code >= 51 && code <= 59:
		return "Drizzle"
	case code >= 61 && code <= 69:
		return "Rain"
	case code >= 71 && code <= 79:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Rain"
	case code >= 85 && code <= 86:
		return "Snow"
	case code >= 95 && code <= 99:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

// Helper function for more detailed descriptions
func (agent *WeatherAgent) weatherCodeToDescription(code int) string {
	switch {
	case code == 0:
		return "clear sky"
	case code == 1:
		return "mainly clear"
	case code == 2:
		return "partly cloudy"
	case code == 3:
		return "overcast"
	case code == 45:
		return "fog"
	case code == 48:
		return "depositing rime fog"
	case code == 51:
		return "light drizzle"
	case code == 53:
		return "moderate drizzle"
	case code == 55:
		return "dense drizzle"
	case code == 61:
		return "slight rain"
	case code == 63:
		return "moderate rain"
	case code == 65:
		return "heavy rain"
	case code == 71:
		return "slight snow fall"
	case code == 73:
		return "moderate snow fall"
	case code == 75:
		return "heavy snow fall"
	case code == 95:
		return "thunderstorm"
	case code == 96, code == 99:
		return "thunderstorm with hail"
	default:
		return "unknown conditions"
	}
}

// Add this method to your WeatherAgent struct in the main.go file

// Get temperature unit symbol based on config
func (agent *WeatherAgent) getTempUnit() string {
	if agent.config.Units == "imperial" {
		return "°F"
	}
	return "°C"
}

// Get wind speed unit
func (agent *WeatherAgent) getWindUnit() string {
	if agent.config.Units == "imperial" {
		return "mph"
	}
	return "m/s"
}

// Prepare weather data for LLM
// Update prepareWeatherData to include day/night information
// Modify the prepareWeatherData method to fix the time display
// Modify the prepareWeatherData method to be extremely explicit about time
// Helper function to get AQI description based on value for OpenWeatherMap API
func getAQIDescription(aqi int) string {
	switch aqi {
	case 1:
		return "Good (1): Air quality is considered satisfactory, and air pollution poses little or no risk."
	case 2:
		return "Fair (2): Air quality is acceptable; however, for some pollutants there may be a moderate health concern for a very small number of people."
	case 3:
		return "Moderate (3): Members of sensitive groups may experience health effects. The general public is not likely to be affected."
	case 4:
		return "Poor (4): Everyone may begin to experience health effects; members of sensitive groups may experience more serious health effects."
	case 5:
		return "Very Poor (5): Health warnings of emergency conditions. The entire population is more likely to be affected."
	default:
		return fmt.Sprintf("Unknown AQI value: %d", aqi)
	}
}

// Fetch air quality data from IQAir API
func (agent *WeatherAgent) fetchIQAirData(weather *WeatherResponse, lat, lon float64) {
	// IQAir API endpoint - add timestamp to prevent caching
	timestamp := time.Now().UnixNano()
	iqairURL := fmt.Sprintf("https://api.airvisual.com/v2/nearest_city?lat=%.6f&lon=%.6f&key=%s&_t=%d",
		lat, lon, agent.config.IQAirAPIKey, timestamp)
	
	agent.logger.Printf("DEBUG: Fetching AQI data from IQAir: %s", strings.Replace(iqairURL, agent.config.IQAirAPIKey, "[REDACTED]", 1))
	agent.logger.Printf("DEBUG: IQAir API key length: %d, first 4 chars: %s", len(agent.config.IQAirAPIKey), agent.config.IQAirAPIKey[:4])
	
	// Print directly to stdout for debugging
	fmt.Printf("\n==== IQAIR API REQUEST ====\n")
	fmt.Printf("DEBUG: Making IQAir API request with key: %s... (length: %d)\n", 
		agent.config.IQAirAPIKey[:4], len(agent.config.IQAirAPIKey))
	fmt.Printf("DEBUG: Request URL: %s\n", strings.Replace(iqairURL, agent.config.IQAirAPIKey, "[REDACTED]", 1))
	
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	req, _ := http.NewRequest("GET", iqairURL, nil)
	req.Header.Add("User-Agent", "WeatherAgent/1.0")
	// Disable caching
	req.Header.Add("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Add("Pragma", "no-cache")
	
	iqairResp, err := client.Do(req)
	if err != nil {
		agent.logger.Printf("WARNING: Failed to fetch IQAir data: %v", err)
		fmt.Printf("ERROR: Failed to fetch IQAir data: %v\n", err)  // Print directly to stdout
		return
	}
	defer iqairResp.Body.Close()
	
	statusMsg := fmt.Sprintf("DEBUG: IQAir API response status: %d", iqairResp.StatusCode)
	agent.logger.Print(statusMsg)
	fmt.Println(statusMsg)
	
	// Log all response headers for debugging
	fmt.Println("DEBUG: IQAir API response headers:")
	for name, values := range iqairResp.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", name, value)
		}
	}
	
	if iqairResp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("WARNING: IQAir API returned status %d", iqairResp.StatusCode)
		agent.logger.Print(errMsg)
		fmt.Println(errMsg)
		return
	}
	
	// Read the response body for logging
	bodyBytes, readErr := io.ReadAll(iqairResp.Body)
	if readErr != nil {
		errMsg := fmt.Sprintf("WARNING: Failed to read IQAir response body: %v", readErr)
		agent.logger.Print(errMsg)
		fmt.Println(errMsg)
		return
	}
	
	// Log response body to both logger and stdout
	responseBody := string(bodyBytes)
	agent.logger.Printf("DEBUG: IQAir API response body: %s", responseBody)
	fmt.Printf("DEBUG: IQAir API response body: %s\n", responseBody)
	
	// Parse the IQAir response
	var iqairResponse struct {
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
					Ts      string  `json:"ts"`
					Tp      float64 `json:"tp"`
					Pr      float64 `json:"pr"`
					Hu      float64 `json:"hu"`
					Ws      float64 `json:"ws"`
					Wd      float64 `json:"wd"`
					Ic      string  `json:"ic"`
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
	
	// Create a new reader with the same data for decoding
	bodyReader := bytes.NewReader(bodyBytes)
	
	if err := json.NewDecoder(bodyReader).Decode(&iqairResponse); err != nil {
		errMsg := fmt.Sprintf("WARNING: Failed to decode IQAir data: %v", err)
		agent.logger.Print(errMsg)
		fmt.Println(errMsg)
		return
	}
	
	if iqairResponse.Status != "success" {
		errMsg := fmt.Sprintf("WARNING: IQAir API returned status %s", iqairResponse.Status)
		agent.logger.Print(errMsg)
		fmt.Println(errMsg)
		return
	}
	
	fmt.Println("DEBUG: Successfully parsed IQAir API response with status: success")
	
	// Get AQI category based on US AQI value
	aqi := iqairResponse.Data.Current.Pollution.Aqius
	var category string
	switch {
	case aqi <= 50:
		category = "Good"
	case aqi <= 100:
		category = "Moderate"
	case aqi <= 150:
		category = "Unhealthy for Sensitive Groups"
	case aqi <= 200:
		category = "Unhealthy"
	case aqi <= 300:
		category = "Very Unhealthy"
	default:
		category = "Hazardous"
	}
	
	// Get pollutant unit based on main pollutant
	pollutant := iqairResponse.Data.Current.Pollution.Mainus
	var pollutantUnit string
	var pollutantValue float64
	
	switch pollutant {
	case "p2":
		pollutantUnit = "μg/m³"
		pollutantValue = iqairResponse.Data.Current.Pollution.P2
	case "p1":
		pollutantUnit = "μg/m³"
		pollutantValue = iqairResponse.Data.Current.Pollution.P1
	case "o3":
		pollutantUnit = "ppb"
		pollutantValue = iqairResponse.Data.Current.Pollution.O3
	case "n2":
		pollutantUnit = "ppb"
		pollutantValue = iqairResponse.Data.Current.Pollution.N2
	case "s2":
		pollutantUnit = "ppb"
		pollutantValue = iqairResponse.Data.Current.Pollution.S2
	case "co":
		pollutantUnit = "ppm"
		pollutantValue = iqairResponse.Data.Current.Pollution.Co
	default:
		pollutantUnit = "μg/m³"
		pollutantValue = 0
	}
	
	// Map pollutant code to human-readable name
	var pollutantName string
	switch pollutant {
	case "p2":
		pollutantName = "PM2.5"
	case "p1":
		pollutantName = "PM10"
	case "o3":
		pollutantName = "Ozone"
	case "n2":
		pollutantName = "Nitrogen Dioxide"
	case "s2":
		pollutantName = "Sulfur Dioxide"
	case "co":
		pollutantName = "Carbon Monoxide"
	default:
		pollutantName = pollutant
	}
	
	// Add IQAir data to the weather response
	weather.IQAirData = struct {
		AQI            int     `json:"aqi"`
		Category       string  `json:"category"`
		PollutantName  string  `json:"pollutant_name"`
		PollutantValue float64 `json:"pollutant_value"`
		PollutantUnit  string  `json:"pollutant_unit"`
		PM25           float64 `json:"pm25"`
		PM10           float64 `json:"pm10"`
	}{
		AQI:            aqi,
		Category:       category,
		PollutantName:  pollutantName,
		PollutantValue: pollutantValue,
		PollutantUnit:  pollutantUnit,
		PM25:           iqairResponse.Data.Current.Pollution.P2,
		PM10:           iqairResponse.Data.Current.Pollution.P1,
	}
	
	successMsg := fmt.Sprintf("Successfully added IQAir AQI data: %d (%s)", aqi, category)
	agent.logger.Print(successMsg)
	fmt.Println(successMsg)
	fmt.Println("==== IQAIR API REQUEST COMPLETE ====\n")
	
	// Log to a special file just for IQAir API calls
	logFile, err := os.OpenFile("iqair_api_calls.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer logFile.Close()
		timestamp := time.Now().Format(time.RFC3339)
		logFile.WriteString(fmt.Sprintf("[%s] IQAir API call: lat=%.6f, lon=%.6f, status=%s, AQI=%d, Category=%s\n", 
			timestamp, lat, lon, "success", aqi, category))
	}
}

func (agent *WeatherAgent) prepareWeatherData(weather WeatherResponse) map[string]interface{} {
	// Create the timezone for the location
	locationTimezone := time.FixedZone("Local", weather.Timezone)
	// Convert the stored Unix timestamp to the proper timezone
	localTime := time.Unix(weather.Dt, 0).In(locationTimezone)

	// Get sunrise/sunset in local time if available
	var sunrise, sunset string
	var dayLength float64
	if weather.Sys.Sunrise > 0 && weather.Sys.Sunset > 0 {
		// Convert sunrise to location timezone
		sunriseTime := time.Unix(weather.Sys.Sunrise, 0).In(locationTimezone)
		sunrise = sunriseTime.Format("3:04 PM")
		
		// Convert sunset to location timezone
		sunsetTime := time.Unix(weather.Sys.Sunset, 0).In(locationTimezone)
		sunset = sunsetTime.Format("3:04 PM")
		
		// Calculate day length in hours and minutes
		dayLengthSeconds := weather.Sys.Sunset - weather.Sys.Sunrise
		dayLength = float64(dayLengthSeconds) / 3600.0
	}

	// Weather condition
	condition := ""
	description := ""
	weatherId := 0
	if len(weather.Weather) > 0 {
		condition = weather.Weather[0].Main
		description = weather.Weather[0].Description
		weatherId = weather.Weather[0].ID
	}

	// Determine if it's day or night based on the time
	hour := localTime.Hour()
	isDaytime := hour >= 6 && hour < 20 // Simple approximation if we don't have actual sunrise/sunset
	
	// More accurate day/night calculation if we have sunrise/sunset
	if weather.Sys.Sunrise > 0 && weather.Sys.Sunset > 0 {
		currentUnix := localTime.Unix()
		isDaytime = currentUnix >= weather.Sys.Sunrise && currentUnix < weather.Sys.Sunset
	}
	
	dayNightString := "DAYTIME"
	if !isDaytime {
		dayNightString = "NIGHTTIME"
	}

	// Format times in multiple ways for absolute clarity
	time12h := localTime.Format("3:04 PM")
	time24h := localTime.Format("15:04")
	timeWithSeconds := localTime.Format("3:04:05 PM")
	fullTimeDate := localTime.Format("Monday, January 2, 2006 at 3:04 PM")
	
	// Calculate moon phase (simplified approximation)
	// Get days since new moon on Jan 6, 2000
	daysSinceNewMoon := (localTime.Unix() - 947182440) / 86400
	// Moon phase cycles every 29.53 days
	moonAge := float64(daysSinceNewMoon % 30) / 29.53
	
	var moonPhase string
	switch {
	case moonAge < 0.025 || moonAge >= 0.975: // 0-2.5% or 97.5-100%
		moonPhase = "New Moon"
	case moonAge < 0.225: // 2.5-22.5%
		moonPhase = "Waxing Crescent"
	case moonAge < 0.275: // 22.5-27.5%
		moonPhase = "First Quarter"
	case moonAge < 0.475: // 27.5-47.5%
		moonPhase = "Waxing Gibbous"
	case moonAge < 0.525: // 47.5-52.5%
		moonPhase = "Full Moon"
	case moonAge < 0.725: // 52.5-72.5%
		moonPhase = "Waning Gibbous"
	case moonAge < 0.775: // 72.5-77.5%
		moonPhase = "Last Quarter"
	default: // 77.5-97.5%
		moonPhase = "Waning Crescent"
	}
	
	// Get wind direction as cardinal/intercardinal point
	windDegree := float64(weather.Wind.Deg)
	windDirection := ""
	switch {
	case windDegree >= 337.5 || windDegree < 22.5:
		windDirection = "N"
	case windDegree >= 22.5 && windDegree < 67.5:
		windDirection = "NE"
	case windDegree >= 67.5 && windDegree < 112.5:
		windDirection = "E"
	case windDegree >= 112.5 && windDegree < 157.5:
		windDirection = "SE"
	case windDegree >= 157.5 && windDegree < 202.5:
		windDirection = "S"
	case windDegree >= 202.5 && windDegree < 247.5:
		windDirection = "SW"
	case windDegree >= 247.5 && windDegree < 292.5:
		windDirection = "W"
	case windDegree >= 292.5 && windDegree < 337.5:
		windDirection = "NW"
	}
	
	// Calculate heat index if temperature > 80°F (26.7°C) and humidity > 40%
	var heatIndex float64
	tempF := weather.Main.Temp
	if agent.config.Units == "metric" {
		tempF = weather.Main.Temp*9/5 + 32 // Convert to Fahrenheit for calculation
	}
	if tempF > 80 && weather.Main.Humidity > 40 {
		// Rothfusz formula
		heatIndex = -42.379 + 2.04901523*tempF + 10.14333127*float64(weather.Main.Humidity) - 
			0.22475541*tempF*float64(weather.Main.Humidity) - 0.00683783*tempF*tempF - 
			0.05481717*float64(weather.Main.Humidity)*float64(weather.Main.Humidity) + 
			0.00122874*tempF*tempF*float64(weather.Main.Humidity) + 
			0.00085282*tempF*float64(weather.Main.Humidity)*float64(weather.Main.Humidity) - 
			0.00000199*tempF*tempF*float64(weather.Main.Humidity)*float64(weather.Main.Humidity)
		
		// Convert back to Celsius if needed
		if agent.config.Units == "metric" {
			heatIndex = (heatIndex - 32) * 5 / 9
		}
	}
	
	// Format visibility
	visibilityStr := "Unknown"
	// Debug visibility value
	agent.logger.Printf("DEBUG: Visibility value from API: %d meters", weather.Visibility)
	
	// OpenWeatherMap returns visibility in meters, and 10000 is their default maximum
	// Visibility might not be in the response or might be 0, default to a reasonable value
	// when it's missing or invalid
	if weather.Visibility <= 0 {
		agent.logger.Printf("WARNING: Visibility is missing, zero or negative: %d - using default value", weather.Visibility)
		// Set a default visibility value (assuming good visibility) if missing
		weather.Visibility = 10000
	}
	
	if agent.config.Units == "metric" {
		visibilityStr = fmt.Sprintf("%.1f km", float64(weather.Visibility)/1000)
	} else {
		visibilityStr = fmt.Sprintf("%.1f miles", float64(weather.Visibility)/1609.34)
	}
	
	// If the visibility is at the API's default maximum (10000 meters)
	if weather.Visibility == 10000 {
		if agent.config.Units == "metric" {
			visibilityStr = "10+ km (excellent)"
		} else {
			visibilityStr = "6.2+ miles (excellent)"
		}
	}

	// Create a map of the current weather data
	data := map[string]interface{}{
		"city":                  weather.Name,
		"country":               weather.Sys.Country,
		"current_local_time":    time12h + " (" + time24h + " in 24-hour format)",
		"time_12h":              time12h,
		"time_24h":              time24h,
		"time_with_seconds":     timeWithSeconds,
		"full_date_and_time":    fullTimeDate,
		"day_of_week":           localTime.Weekday().String(),
		"hour_of_day":           hour,
		"is_daytime_or_night":   dayNightString,
		"date":                  localTime.Format("January 2, 2006"),
		"temperature":           fmt.Sprintf("%.1f%s", weather.Main.Temp, agent.getTempUnit()),
		"feels_like":            fmt.Sprintf("%.1f%s", weather.Main.FeelsLike, agent.getTempUnit()),
		"temp_min":              fmt.Sprintf("%.1f%s", weather.Main.TempMin, agent.getTempUnit()),
		"temp_max":              fmt.Sprintf("%.1f%s", weather.Main.TempMax, agent.getTempUnit()),
		"condition":             condition,
		"description":           description,
		"weather_id":            weatherId,
		"humidity":              weather.Main.Humidity,
		"pressure":              fmt.Sprintf("%d hPa", weather.Main.Pressure),
		"wind_speed":            fmt.Sprintf("%.1f %s", weather.Wind.Speed, agent.getWindUnit()),
		"wind_direction":        weather.Wind.Deg,
		"wind_direction_text":   windDirection,
		"wind_gust":             fmt.Sprintf("%.1f %s", weather.Wind.Gust, agent.getWindUnit()),
		"visibility":            visibilityStr,
		"cloud_cover":           fmt.Sprintf("%d%%", weather.Clouds.All),
		"sunrise":               sunrise,
		"sunset":                sunset,
		"day_length":            fmt.Sprintf("%.1f hours", dayLength),
		"moon_phase":            moonPhase,
		"units":                 agent.config.Units,
		"is_daytime":            isDaytime,
		"timezone_offset_hours": weather.Timezone / 3600,
		"timezone_name":         fmt.Sprintf("UTC%+d", weather.Timezone/3600),
	}
	
	// Log raw visibility value from API for debugging
	agent.logger.Printf("Raw visibility value from API response: %d meters", weather.Visibility)
	
	// Check for IQAir data first, then fall back to OpenWeatherMap AQI data
	if weather.IQAirData.AQI > 0 {
		agent.logger.Printf("DEBUG: Using IQAir AQI data")
		
		data["aqi"] = weather.IQAirData.AQI
		data["aqi_description"] = weather.IQAirData.Category
		data["aqi_source"] = "IQAir"
		
		// Add individual pollutant data
		data["pollutant_name"] = weather.IQAirData.PollutantName
		data["pollutant_value"] = fmt.Sprintf("%.1f %s", weather.IQAirData.PollutantValue, weather.IQAirData.PollutantUnit)
		data["pm2_5"] = fmt.Sprintf("%.1f μg/m³", weather.IQAirData.PM25)
		data["pm10"] = fmt.Sprintf("%.1f μg/m³", weather.IQAirData.PM10)
		
		agent.logger.Printf("Added IQAir AQI data: %d (%s)", weather.IQAirData.AQI, weather.IQAirData.Category)
	} else if len(weather.AQI.List) > 0 {
		// Fallback to OpenWeatherMap AQI data
		agent.logger.Printf("DEBUG: Using OpenWeatherMap AQI data. AQI list length: %d", len(weather.AQI.List))
		aqiValue := weather.AQI.List[0].Main.AQI
		aqiDesc := getAQIDescription(aqiValue)
		
		data["aqi"] = aqiValue
		data["aqi_description"] = aqiDesc
		data["aqi_source"] = "OpenWeatherMap"
		
		// Add individual pollutant data
		components := weather.AQI.List[0].Components
		data["co"] = fmt.Sprintf("%.1f μg/m³", components.CO)
		data["no2"] = fmt.Sprintf("%.1f μg/m³", components.NO2)
		data["o3"] = fmt.Sprintf("%.1f μg/m³", components.O3)
		data["so2"] = fmt.Sprintf("%.1f μg/m³", components.SO2)
		data["pm2_5"] = fmt.Sprintf("%.1f μg/m³", components.PM2_5)
		data["pm10"] = fmt.Sprintf("%.1f μg/m³", components.PM10)
		
		agent.logger.Printf("Added OpenWeatherMap AQI data: %d (%s)", aqiValue, aqiDesc)
	} else {
		agent.logger.Printf("No AQI data available from any source")
	}
	
	// DEBUG: Print all data being sent to the frontend
	agent.logger.Printf("DEBUG: Full weather data map being sent to frontend:")
	for k, v := range data {
		agent.logger.Printf("  %s: %v", k, v)
	}
	
	// Add heat index if calculated
	if heatIndex > 0 {
		data["heat_index"] = fmt.Sprintf("%.1f%s", heatIndex, agent.getTempUnit())
	}

	// Add rain data if available
	if weather.Rain.OneHour > 0 {
		data["rain_1h"] = fmt.Sprintf("%.1f mm", weather.Rain.OneHour)
	}
	if weather.Rain.ThreeHours > 0 {
		data["rain_3h"] = fmt.Sprintf("%.1f mm", weather.Rain.ThreeHours)
	}

	// Add snow data if available
	if weather.Snow.OneHour > 0 {
		data["snow_1h"] = fmt.Sprintf("%.1f mm", weather.Snow.OneHour)
	}
	if weather.Snow.ThreeHours > 0 {
		data["snow_3h"] = fmt.Sprintf("%.1f mm", weather.Snow.ThreeHours)
	}

	// Time display for UI - ensure we have a time field specifically for the UI
	data["time"] = time12h // This is what displays in the UI

	// Log all time-related fields for debugging
	agent.logger.Printf("TIME FIELDS FOR UI:")
	agent.logger.Printf("time: %s", data["time"])
	agent.logger.Printf("time_12h: %s", data["time_12h"])
	agent.logger.Printf("time_24h: %s", data["time_24h"])
	agent.logger.Printf("current_local_time: %s", data["current_local_time"])

	return data
}

// Generate message using LLM API
// Modify the generateLLMMessage function to explicitly address the time issue
// Add this to the beginning of the generateLLMMessage function
func (agent *WeatherAgent) generateLLMMessage(currentWeather WeatherResponse, historyContext string) (string, error) {
	// Debug the timestamp and timezone before any processing
	agent.logger.Printf("======= LLM MESSAGE TIME DEBUG =======")
	agent.logger.Printf("Unix timestamp: %d", currentWeather.Dt)
	agent.logger.Printf("Timezone offset: %d seconds (%d hours)",
		currentWeather.Timezone, currentWeather.Timezone/3600)

	// Create timezone and get local time
	locationTimezone := time.FixedZone("Local", currentWeather.Timezone)
	localTime := time.Unix(currentWeather.Dt, 0).In(locationTimezone)
	agent.logger.Printf("LOCAL TIME (in location timezone): %s", localTime.Format("15:04:05 MST"))
	agent.logger.Printf("==================================")

	// Continue with the rest of the function
	weatherData := agent.prepareWeatherData(currentWeather)
	time12h := localTime.Format("3:04 PM")
	time24h := localTime.Format("15:04")

	// EXPLICITLY force the LLM to understand the correct time
	timeInstructions := fmt.Sprintf(`IMPORTANT TIME INFORMATION: 
The CURRENT LOCAL TIME in %s is %s (%s in 24-hour format).
This is the accurate local time for this location.
DO NOT convert or adjust this time. It is already the correct local time.
You MUST use this exact time in your weather message.
`,
		currentWeather.Name,
		time12h,
		time24h)

	// Convert weatherData to a formatted string
	var weatherInfo strings.Builder
	weatherInfo.WriteString("Current Weather Data:\n")

	// Add the explicit time instruction first to emphasize its importance
	weatherInfo.WriteString(timeInstructions)
	weatherInfo.WriteString("\n")

	// Add all the weather data
	for key, value := range weatherData {
		weatherInfo.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}

	// Create the user message with current weather data and history context
	userMessage := weatherInfo.String()
	if historyContext != "" {
		userMessage = userMessage + "\n\nWeather history context:\n" + historyContext
	}

	// Add VERY explicit instruction for what kind of response we want
	// Add this to the LLM prompt to make sure the correct units are used
	userMessage += fmt.Sprintf(`
Based on this weather data, generate a helpful, informative, and engaging message about the current weather. Make it natural and conversational.

Consider all the weather details provided, such as temperature, humidity, wind, precipitation, visibility, cloud cover, air quality, and astronomical information when relevant. If there are any notable weather conditions (extreme temperatures, storms, poor air quality, etc.), highlight those.

You can mention interesting weather facts or patterns if they're relevant to the current conditions. For example, if it's a full moon on a clear night, or if it's an unusually warm/cold day for the season.

If air quality information is provided, include health recommendations based on the AQI level.

CRITICAL: The current local time in %s is %s. DO NOT modify or reinterpret this time. Reference this EXACT time in your response.`, currentWeather.Name, time12h)

	// Call the appropriate LLM API based on configuration
	switch strings.ToLower(agent.config.LLMProvider) {
	case "anthropic":
		return agent.callAnthropicAPI(userMessage)
	case "openai":
		return agent.callOpenAIAPI(userMessage)
	default:
		return "", fmt.Errorf("unsupported LLM provider: %s", agent.config.LLMProvider)
	}
}

// Call the Anthropic API (Claude) - updated to current API format
func (agent *WeatherAgent) callAnthropicAPI(userMessage string) (string, error) {
	url := "https://api.anthropic.com/v1/messages"

	// Create request with updated format
	reqBody := struct {
		Model       string             `json:"model"`
		System      string             `json:"system"`
		Messages    []AnthropicMessage `json:"messages"`
		Temperature float64            `json:"temperature"`
		MaxTokens   int                `json:"max_tokens"`
	}{
		Model:  agent.config.LLMModel,
		System: agent.config.SystemPrompt,
		Messages: []AnthropicMessage{
			{
				Role:    "user",
				Content: userMessage,
			},
		},
		Temperature: agent.config.LLMTemperature,
		MaxTokens:   500,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", agent.config.LLMAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Log the raw response for debugging
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var result AnthropicResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("Error parsing response: %v\nResponse: %s", err, string(bodyBytes))
	}

	// Extract message content
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}

	return "", fmt.Errorf("no content in response: %s", string(bodyBytes))
}

// Call the OpenAI API (GPT models)
func (agent *WeatherAgent) callOpenAIAPI(userMessage string) (string, error) {
	url := "https://api.openai.com/v1/chat/completions"

	// Create request
	reqBody := OpenAIRequest{
		Model: agent.config.LLMModel,
		Messages: []OpenAIMessage{
			{
				Role:    "system",
				Content: agent.config.SystemPrompt,
			},
			{
				Role:    "user",
				Content: userMessage,
			},
		},
		Temperature: agent.config.LLMTemperature,
		MaxTokens:   500,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+agent.config.LLMAPIKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var result OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Extract message content
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no content in response")
}

// Generate weather history context
func (agent *WeatherAgent) generateHistoryContext() string {
	if len(agent.weatherHistory) <= 1 {
		return "" // Not enough history yet
	}

	var context strings.Builder

	// Add the previous weather entry
	prevWeather := agent.weatherHistory[len(agent.weatherHistory)-2]
	prevLocationTimezone := time.FixedZone("Local", prevWeather.Timezone)
	prevTime := time.Unix(prevWeather.Dt, 0).In(prevLocationTimezone)

	context.WriteString(fmt.Sprintf("Previous weather (%s):\n", prevTime.Format("15:04")))

	// Add relevant previous weather details
	if len(prevWeather.Weather) > 0 {
		context.WriteString(fmt.Sprintf("- Condition: %s (%s)\n",
			prevWeather.Weather[0].Main, prevWeather.Weather[0].Description))
	}
	context.WriteString(fmt.Sprintf("- Temperature: %.1f%s (feels like %.1f%s)\n",
		prevWeather.Main.Temp, agent.getTempUnit(),
		prevWeather.Main.FeelsLike, agent.getTempUnit()))
	context.WriteString(fmt.Sprintf("- Humidity: %d%%\n", prevWeather.Main.Humidity))
	context.WriteString(fmt.Sprintf("- Wind: %.1f %s\n",
		prevWeather.Wind.Speed, agent.getWindUnit()))

	return context.String()
}

// Update weather and generate new LLM message
func (agent *WeatherAgent) update() {
	fmt.Println("\n==== STARTING WEATHER UPDATE ====")
	fmt.Printf("Time: %s\n", time.Now().Format(time.RFC3339))
	
	weather, err := agent.fetchWeather()
	if err != nil {
		agent.logger.Printf("Error fetching weather: %v", err)
		fmt.Printf("Error fetching weather: %v\n", err)
		return
	}

	// Add to history
	agent.weatherHistory = append(agent.weatherHistory, weather)

	// Keep history to a reasonable size
	if len(agent.weatherHistory) > 24 {
		agent.weatherHistory = agent.weatherHistory[1:]
	}

	// Generate history context
	historyContext := agent.generateHistoryContext()

	// Generate message using LLM
	message, err := agent.generateLLMMessage(weather, historyContext)
	if err != nil {
		agent.logger.Printf("Error generating LLM message: %v", err)
		return
	}

	// Check if the message is too similar to the last one
	if strings.TrimSpace(message) == strings.TrimSpace(agent.lastMessage) {
		agent.logger.Printf("LLM generated identical message, adding variation request and retrying")

		// Add a request for variation
		variedMessage, err := agent.generateLLMMessage(weather,
			historyContext+"\nIMPORTANT: Please generate a completely different message than before.")

		if err == nil && strings.TrimSpace(variedMessage) != strings.TrimSpace(agent.lastMessage) {
			message = variedMessage
		} else {
			// If failed to get variation, add a timestamp to make it different
			currentTime := time.Unix(weather.Dt, 0).UTC().Add(time.Second * time.Duration(weather.Timezone))
			message = fmt.Sprintf("[%s] %s", currentTime.Format("15:04"), message)
		}
	}

	// Log the message
	timeStr := time.Now().Format("15:04:05")
	agent.logger.Printf("[%s] %s\n", timeStr, message)

	// Update last message
	agent.lastMessage = message
	agent.lastMessageTime = time.Now()
}

// Modify the loadConfig function to remove hardcoded secrets
func loadConfig() Config {
	// Log available environment variables for debugging
	fmt.Println("DEBUG: Environment variables:")
	fmt.Printf("DEBUG: IQAIR_API_KEY set: %t\n", os.Getenv("IQAIR_API_KEY") != "")
	fmt.Printf("DEBUG: LLM_API_KEY set: %t\n", os.Getenv("LLM_API_KEY") != "")
	
	config := Config{
		WeatherAPIKey:  getEnv("WEATHER_API_KEY", "not-needed"), // Open-Meteo doesn't need an API key
		LLMAPIKey:      getEnv("LLM_API_KEY", ""),               // Never hardcode API keys
		IQAirAPIKey:    getEnv("IQAIR_API_KEY", ""),             // IQAir API key for air quality data
		City:           getEnv("WEATHER_CITY", "London"),
		CountryCode:    getEnv("WEATHER_COUNTRY", "uk"),
		CheckInterval:  getEnvInt("WEATHER_CHECK_INTERVAL", 1),
		Units:          getEnv("WEATHER_UNITS", "metric"), // metric or imperial
		LogToFile:      getEnvBool("WEATHER_LOG_TO_FILE", false),
		LogFile:        getEnv("WEATHER_LOG_FILE", "weather.log"),
		LLMProvider:    getEnv("LLM_PROVIDER", "anthropic"),
		LLMModel:       getEnv("LLM_MODEL", "claude-3-haiku-20240307"),
		LLMTemperature: getEnvFloat("LLM_TEMPERATURE", 0.7),
		SystemPrompt:   getEnv("LLM_SYSTEM_PROMPT", ""),
	}

	// Validate LLM model based on provider
	if config.LLMProvider == "anthropic" && !strings.Contains(config.LLMModel, "claude") {
		// Default to Claude if not specified properly
		config.LLMModel = "claude-3-haiku-20240307"
	} else if config.LLMProvider == "openai" && !strings.Contains(config.LLMModel, "gpt") {
		// Default to GPT if not specified properly
		config.LLMModel = "gpt-3.5-turbo"
	}

	// Override with command line arguments if provided
	args := os.Args[1:]
	if len(args) >= 1 && args[0] != "" {
		config.City = args[0]
	}

	if len(args) >= 2 && args[1] != "" {
		config.CountryCode = args[1]
	}

	return config
}

// Add this function to help with loading secrets from a file
func loadSecretsFromFile(filename string) {
	fmt.Printf("DEBUG: Attempting to load secrets from file: %s\n", filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		// File doesn't exist or can't be read - just continue with env vars
		fmt.Printf("DEBUG: Error loading secrets file: %v\n", err)
		return
	}
	fmt.Printf("DEBUG: Successfully loaded secrets file, size: %d bytes\n", len(data))
	
	// Check if file contains IQAIR_API_KEY
	if strings.Contains(string(data), "IQAIR_API_KEY") {
		fmt.Println("DEBUG: IQAIR_API_KEY found in secrets file")
	} else {
		fmt.Println("WARNING: IQAIR_API_KEY not found in secrets file")
	}

	lines := strings.Split(string(data), "\n")
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

		// Only set if not already set via environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
			fmt.Printf("DEBUG: Set environment variable from .env file: %s\n", key)
		} else {
			fmt.Printf("DEBUG: Environment variable already set: %s\n", key)
		}
	}
}

// Helper function to get environment variable with default
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// Helper function to get integer environment variable
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	var intValue int
	if _, err := fmt.Sscanf(value, "%d", &intValue); err != nil {
		return defaultValue
	}

	return intValue
}

// Helper function to get float environment variable
func getEnvFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	var floatValue float64
	if _, err := fmt.Sscanf(value, "%f", &floatValue); err != nil {
		return defaultValue
	}

	return floatValue
}

// Helper function to get boolean environment variable
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return strings.ToLower(value) == "true" || value == "1"
}

// Add this function to your WeatherAgent struct
func (agent *WeatherAgent) debugTimeInfo(weather WeatherResponse) {
	// Show server's local time zone
	serverLocation, _ := time.LoadLocation("Local")
	serverTZ := time.Now().In(serverLocation).Format("MST")

	// Get the local time with proper timezone
	locationTimezone := time.FixedZone("Local", weather.Timezone)
	localTime := time.Unix(weather.Dt, 0).In(locationTimezone)

	agent.logger.Printf("======= TIME DEBUG INFO =======")
	agent.logger.Printf("Server timezone: %s", serverTZ)
	agent.logger.Printf("Unix timestamp from API: %d", weather.Dt)
	agent.logger.Printf("Weather location timezone offset: %d seconds (%d hours)", weather.Timezone, weather.Timezone/3600)
	agent.logger.Printf("Local time at weather location: %s", localTime.Format(time.RFC3339))
	agent.logger.Printf("==============================")
}

// Modify the main function to load secrets before config
func testIQAirAPI(apiKey string) {
	if apiKey == "" {
		fmt.Println("ERROR: IQAir API key is empty")
		return
	}
	
	fmt.Printf("Testing IQAir API with key: %s (length: %d)\n", apiKey[:4] + "...", len(apiKey))
	
	// Test with New York coordinates
	lat, lon := 40.7128, -74.0060
	
	iqairURL := fmt.Sprintf("https://api.airvisual.com/v2/nearest_city?lat=%.6f&lon=%.6f&key=%s",
		lat, lon, apiKey)
	
	fmt.Printf("DEBUG: IQAir API URL: %s\n", strings.Replace(iqairURL, apiKey, "[REDACTED]", 1))
	
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	
	req, _ := http.NewRequest("GET", iqairURL, nil)
	req.Header.Add("User-Agent", "WeatherAgent/1.0")
	
	fmt.Println("DEBUG: Sending request to IQAir API...")
	
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("ERROR: Failed to call IQAir API: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	fmt.Printf("DEBUG: IQAir API response status: %d\n", resp.StatusCode)
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read response body: %v\n", err)
		return
	}
	
	fmt.Printf("DEBUG: IQAir API response body: %s\n", string(bodyBytes))
	
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		fmt.Printf("ERROR: Failed to parse JSON response: %v\n", err)
		return
	}
	
	fmt.Printf("DEBUG: Parsed response: %+v\n", result)
}

func main() {
	// Load secrets and config as before
	loadSecretsFromFile(".env")
	config := loadConfig()

	// Test IQAir API directly
	fmt.Println("=====================")
	fmt.Println("TESTING IQAIR API")
	fmt.Println("=====================")
	testIQAirAPI(config.IQAirAPIKey)
	fmt.Println("=====================")

	// Check for required API key
	if config.LLMAPIKey == "" {
		fmt.Println("LLM API key not set. Please set LLM_API_KEY environment variable or add it to a .env file.")
		fmt.Println("You can create a .env file with your API key like this:")
		fmt.Println("LLM_API_KEY=your_api_key_here")
		os.Exit(1)
	}

	// Create our AI agent
	agent := NewWeatherAgent(config)

	// Helper function to generate fresh weather data and message
	generateWeatherUpdate := func() (string, string, string, string, map[string]interface{}, error) {
		// Get current city/country from environment (might have been updated)
		currentCity := getEnv("WEATHER_CITY", config.City)
		currentCountry := getEnv("WEATHER_COUNTRY", config.CountryCode)

		// Update agent config
		agent.config.City = currentCity
		agent.config.CountryCode = currentCountry

		// Get weather update
		weather, err := agent.fetchWeather()
		if err != nil {
			return "", "", "", "", nil, fmt.Errorf("error fetching weather: %v", err)
		}

		// Add to history for context
		agent.weatherHistory = append(agent.weatherHistory, weather)
		if len(agent.weatherHistory) > 24 {
			agent.weatherHistory = agent.weatherHistory[1:]
		}

		// Generate weather message
		historyContext := agent.generateHistoryContext()
		message, err := agent.generateLLMMessage(weather, historyContext)
		if err != nil {
			return "", "", "", "", nil, fmt.Errorf("error generating LLM message: %v", err)
		}

		// Prepare weather data
		weatherData := agent.prepareWeatherData(weather)
		timeStr := time.Now().Format(time.RFC1123)

		// Log the message
		agent.logger.Printf("[%s] Generated fresh weather message for %s: %s",
			time.Now().Format("15:04:05"), currentCity, message)

		return message, currentCity, currentCountry, timeStr, weatherData, nil
	}

	// Helper function to generate weather data using coordinates instead of city name
	generateWeatherUpdateByCoordinates := func(lat, lon float64) (string, string, string, string, map[string]interface{}, error) {
		// Create a custom weather fetching function for coordinates
		weather, err := agent.fetchWeatherByCoordinates(lat, lon)
		if err != nil {
			return "", "", "", "", nil, fmt.Errorf("error fetching weather by coordinates: %v", err)
		}

		// Add to history for context
		agent.weatherHistory = append(agent.weatherHistory, weather)
		if len(agent.weatherHistory) > 24 {
			agent.weatherHistory = agent.weatherHistory[1:]
		}

		// Generate weather message
		historyContext := agent.generateHistoryContext()
		message, err := agent.generateLLMMessage(weather, historyContext)
		if err != nil {
			return "", "", "", "", nil, fmt.Errorf("error generating LLM message: %v", err)
		}

		// Prepare weather data
		weatherData := agent.prepareWeatherData(weather)
		timeStr := time.Now().Format(time.RFC1123)

		// Log the message
		agent.logger.Printf("[%s] Generated fresh weather message for coordinates (%.4f, %.4f): %s",
			time.Now().Format("15:04:05"), lat, lon, message)

		return message, weather.Name, weather.Sys.Country, timeStr, weatherData, nil
	}

	// Set up HTTP handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve the main HTML page with loading state
		tmpl, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get current city/country from environment
		currentCity := getEnv("WEATHER_CITY", config.City)
		currentCountry := getEnv("WEATHER_COUNTRY", config.CountryCode)

		data := struct {
			City      string
			Country   string
			Message   string
			Timestamp string
		}{
			City:      currentCity,
			Country:   currentCountry,
			Message:   "Loading weather data...",
			Timestamp: "Initializing...",
		}

		tmpl.Execute(w, data)
	})

	http.HandleFunc("/api/update-city", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form: "+err.Error(), http.StatusBadRequest)
			return
		}

		city := r.FormValue("city")
		country := r.FormValue("country")

		if city == "" {
			http.Error(w, "City is required", http.StatusBadRequest)
			return
		}

		// Update environment variables
		os.Setenv("WEATHER_CITY", city)
		if country != "" {
			os.Setenv("WEATHER_COUNTRY", country)
		}

		// Redirect back to home page
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// API endpoint to get fresh weather data
	http.HandleFunc("/api/weather", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("\n==== RECEIVED REQUEST TO /api/weather ENDPOINT ====\n")
		fmt.Printf("Time: %s\n", time.Now().Format(time.RFC3339))
		fmt.Printf("Remote address: %s\n", r.RemoteAddr)
		fmt.Printf("User agent: %s\n", r.UserAgent())
		
		// Check if coordinates are provided in query parameters
		latParam := r.URL.Query().Get("lat")
		lonParam := r.URL.Query().Get("lon")

		var message, city, country, timestamp string
		var weatherData map[string]interface{}
		var err error

		if latParam != "" && lonParam != "" {
			// Parse coordinates
			lat, err1 := strconv.ParseFloat(latParam, 64)
			lon, err2 := strconv.ParseFloat(lonParam, 64)

			if err1 != nil || err2 != nil {
				http.Error(w, "Invalid coordinates", http.StatusBadRequest)
				return
			}

			// Generate weather update using coordinates
			message, city, country, timestamp, weatherData, err = generateWeatherUpdateByCoordinates(lat, lon)
		} else {
			// Generate weather update using configured city
			message, city, country, timestamp, weatherData, err = generateWeatherUpdate()
		}

		if err != nil {
			agent.logger.Printf("Error generating weather update: %v", err)
			http.Error(w, "Unable to fetch weather data", http.StatusInternalServerError)
			return
		}

		// Debug the time data being sent to the browser
		if weatherData != nil {
			log.Printf("TIME DATA SENT TO BROWSER: %v", weatherData["time"])
			
			// Check if AQI data is present
			if aqi, ok := weatherData["aqi"]; ok {
				log.Printf("AQI DATA SENT TO BROWSER: %v", aqi)
			} else {
				log.Printf("WARNING: No AQI data found in weatherData map")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city":      city,
			"country":   country,
			"message":   message,
			"timestamp": timestamp,
			"data":      weatherData,
		})
	})

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Start the HTTP server
	port := getEnv("PORT", "8080")
	fmt.Printf("Starting web server at http://localhost:%s\n", port)
	fmt.Println("Press Ctrl+C to stop")

	http.ListenAndServe(":"+port, nil)
}
