package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Configuration
type Config struct {
	WeatherAPIKey  string
	LLMAPIKey      string
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

	// Parse the local time from the API response - handle both formats
	var localTime time.Time
	timeFormats := []string{
		time.RFC3339,       // Try full RFC3339 format first: "2006-01-02T15:04:05Z07:00"
		"2006-01-02T15:04", // Try format without seconds: "2006-01-02T15:04"
		"2006-01-02 15:04", // Try alternative format
	}

	var parseErr error
	for _, format := range timeFormats {
		localTime, parseErr = time.Parse(format, openMeteoResp.Current.Time)
		if parseErr == nil {
			break // Successfully parsed
		}
	}

	if parseErr != nil {
		// Fall back to current time if parsing fails
		localTime = time.Now()
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
		Dt:       localTime.Unix(),             // Local time from API
		Timezone: openMeteoResp.TimezoneOffset, // Timezone offset in seconds
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

	return weather, nil
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
func (agent *WeatherAgent) prepareWeatherData(weather WeatherResponse) map[string]interface{} {
	// Parse the time from the Unix timestamp using the timezone offset
	localTime := time.Unix(weather.Dt, 0)

	// Get sunrise/sunset in local time if available
	var sunrise, sunset string
	if weather.Sys.Sunrise > 0 {
		sunriseTime := time.Unix(weather.Sys.Sunrise, 0)
		sunrise = sunriseTime.Format("15:04")
	}
	if weather.Sys.Sunset > 0 {
		sunsetTime := time.Unix(weather.Sys.Sunset, 0)
		sunset = sunsetTime.Format("15:04")
	}

	// Weather condition
	condition := ""
	description := ""
	if len(weather.Weather) > 0 {
		condition = weather.Weather[0].Main
		description = weather.Weather[0].Description
	}

	// Determine if it's day or night based on the time
	hour := localTime.Hour()
	isDaytime := hour >= 6 && hour < 20 // Simple approximation if we don't have actual sunrise/sunset

	// Create a map of the current weather data
	data := map[string]interface{}{
		"city":            weather.Name,
		"country":         weather.Sys.Country,
		"time":            localTime.Format("15:04"),
		"date":            localTime.Format("2006-01-02"),
		"day_of_week":     localTime.Weekday().String(),
		"temperature":     fmt.Sprintf("%.1f%s", weather.Main.Temp, agent.getTempUnit()),
		"feels_like":      fmt.Sprintf("%.1f%s", weather.Main.FeelsLike, agent.getTempUnit()),
		"condition":       condition,
		"description":     description,
		"humidity":        weather.Main.Humidity,
		"wind_speed":      fmt.Sprintf("%.1f %s", weather.Wind.Speed, agent.getWindUnit()),
		"wind_direction":  weather.Wind.Deg,
		"visibility":      weather.Visibility,
		"sunrise":         sunrise,
		"sunset":          sunset,
		"units":           agent.config.Units,
		"is_daytime":      isDaytime,
		"local_time":      localTime.Format(time.RFC3339),
		"timezone_offset": weather.Timezone,
	}

	// Add rain data if available
	if weather.Rain.OneHour > 0 {
		data["rain_1h"] = weather.Rain.OneHour
	}
	if weather.Rain.ThreeHours > 0 {
		data["rain_3h"] = weather.Rain.ThreeHours
	}

	// Add snow data if available
	if weather.Snow.OneHour > 0 {
		data["snow_1h"] = weather.Snow.OneHour
	}
	if weather.Snow.ThreeHours > 0 {
		data["snow_3h"] = weather.Snow.ThreeHours
	}

	return data
}

// Generate message using LLM API
func (agent *WeatherAgent) generateLLMMessage(currentWeather WeatherResponse, historyContext string) (string, error) {
	weatherData := agent.prepareWeatherData(currentWeather)

	// Convert weatherData to a formatted string
	var weatherInfo strings.Builder
	weatherInfo.WriteString("Current Weather Data:\n")
	for key, value := range weatherData {
		weatherInfo.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}

	// Create the user message with current weather data and history context
	userMessage := weatherInfo.String()
	if historyContext != "" {
		userMessage = userMessage + "\n\nWeather history context:\n" + historyContext
	}

	// Add instruction for what kind of response we want
	userMessage += "\n\nBased on this weather data, generate a helpful, informative, and engaging message about the current weather. Make it natural and conversational."

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
	prevTime := time.Unix(prevWeather.Dt, 0).UTC().Add(time.Second * time.Duration(prevWeather.Timezone))

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
	weather, err := agent.fetchWeather()
	if err != nil {
		agent.logger.Printf("Error fetching weather: %v", err)
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
	config := Config{
		WeatherAPIKey:  getEnv("WEATHER_API_KEY", "not-needed"), // Open-Meteo doesn't need an API key
		LLMAPIKey:      getEnv("LLM_API_KEY", ""),               // Never hardcode API keys
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
	data, err := os.ReadFile(filename)
	if err != nil {
		// File doesn't exist or can't be read - just continue with env vars
		return
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

// Modify the main function to load secrets before config
func main() {
	// Try to load secrets from .env file if it exists
	loadSecretsFromFile(".env")

	// Load configuration
	config := loadConfig()

	// Check if LLM API key is set
	if config.LLMAPIKey == "" {
		fmt.Println("LLM API key not set. Please set LLM_API_KEY environment variable or add it to a .env file.")
		fmt.Println("You can create a .env file with your API key like this:")
		fmt.Println("LLM_API_KEY=your_api_key_here")
		os.Exit(1)
	}

	// Create our AI agent
	agent := NewWeatherAgent(config)

	fmt.Printf("Weather LLM Agent started for %s, %s\n", config.City, config.CountryCode)
	fmt.Printf("LLM Provider: %s (Model: %s)\n", config.LLMProvider, config.LLMModel)
	fmt.Printf("Check interval: %d minute(s)\n", config.CheckInterval)
	fmt.Printf("Units: %s\n", config.Units)
	fmt.Println("Press Ctrl+C to stop")

	// Initial update
	agent.update()

	// Main monitoring loop
	ticker := time.NewTicker(time.Duration(config.CheckInterval) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		agent.update()
	}
}
