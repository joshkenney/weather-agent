document.addEventListener("DOMContentLoaded", function () {
  // Check if all required elements exist before proceeding
  const weatherDetailsElement = document.getElementById("weatherDetails");
  const weatherMessageElement = document.getElementById("weatherMessage");

  if (!weatherDetailsElement) {
    console.error(
      'Error: Element with ID "weatherDetails" not found in the document',
    );
    return;
  }

  if (!weatherMessageElement) {
    console.error(
      'Error: Element with ID "weatherMessage" not found in the document',
    );
    return;
  }

  // Track last update timestamp to detect changes
  let lastUpdateTimestamp = "";

  // Try to get user's location first, then fetch weather data
  detectLocation();

  // Set up manual refresh button if it exists
  const refreshButton = document.getElementById("refreshButton");
  if (refreshButton) {
    refreshButton.addEventListener("click", function () {
      // Add spinning animation to button icon
      const icon = refreshButton.querySelector("i");
      if (icon) {
        icon.classList.add("refreshing");
      }

      // Disable button during refresh
      refreshButton.disabled = true;

      // Force refresh with location detection
      detectLocation().finally(() => {
        // Re-enable button and remove animation when done
        setTimeout(() => {
          if (icon) {
            icon.classList.remove("refreshing");
          }
          refreshButton.disabled = false;
        }, 1000);
      });
    });
  }

  // Fetch weather data function with silent option
  function fetchWeatherData(silent = false) {
      // Show loading state unless it's a silent refresh
      if (!silent) {
        showLoadingState();
      }

      return fetch("/api/weather")
        .then((response) => {
          if (!response.ok) {
            throw new Error("Network response was not ok");
          }
          console.log("Got response from /api/weather");
          return response.json();
        })
      .then((data) => {
        // Log raw data from API
        console.log("Raw API response:", JSON.stringify(data, null, 2));
        
        // Check if the data has been updated since last fetch
        if (data.timestamp !== lastUpdateTimestamp) {
          console.log("New weather data available! Updating UI...");
          updateWeatherDetails(data);
          updateWeatherMessage(data.message);
          updatePageTitle(data.city, data.country);
          updateTimestamp(data.timestamp);

          // Update our tracked timestamp
          lastUpdateTimestamp = data.timestamp;

          // Show a subtle indicator that data was refreshed
          if (!silent) {
            flashRefreshIndicator();
          }
        } else if (!silent) {
          console.log("No new weather data available yet.");
        }
      })
      .catch((error) => {
        console.error("Error fetching weather data:", error);
        if (!silent) {
          const weatherDetails = document.getElementById("weatherDetails");
          if (weatherDetails) {
            weatherDetails.innerHTML = `<div class="error">Error loading weather data: ${error.message}</div>`;
          }
        }
      });
  }

  // Add this to the global scope so our interval can use it
  window.fetchWeatherData = fetchWeatherData;

  // Show loading state while fetching weather data
  function showLoadingState() {
    const weatherMessage = document.getElementById("weatherMessage");
    const weatherDetails = document.getElementById("weatherDetails");

    if (weatherMessage) {
      weatherMessage.innerHTML = `
                <p class="loading-message">
                    <i class="fas fa-brain"></i>
                    Claude is thinking about the weather...
                </p>
            `;
    }

    if (weatherDetails) {
      weatherDetails.innerHTML = `
                <div class="loading">
                    <i class="fas fa-cloud"></i>
                    Fetching weather data...
                </div>
            `;
    }
  }

  // Detect user location using geolocation API
  function detectLocation() {
    return new Promise((resolve, reject) => {
      if (!navigator.geolocation) {
        console.log(
          "Geolocation is not supported by this browser. Using default location.",
        );
        fetchWeatherData().then(resolve).catch(reject);
        return;
      }

      // Show location detection message
      showLocationDetectionState();

      const geoOptions = {
        timeout: 10000, // 10 seconds timeout
        maximumAge: 300000, // Cache for 5 minutes
        enableHighAccuracy: false, // Don't need high accuracy for weather
      };

      navigator.geolocation.getCurrentPosition(
        // Success callback
        function (position) {
          const lat = position.coords.latitude;
          const lon = position.coords.longitude;
          console.log(`Location detected: ${lat}, ${lon}`);
          fetchWeatherDataByCoordinates(lat, lon).then(resolve).catch(reject);
        },
        // Error callback
        function (error) {
          console.log("Geolocation error:", error.message);
          showLocationError(error);
          // Fall back to default location
          setTimeout(() => {
            fetchWeatherData().then(resolve).catch(reject);
          }, 2000);
        },
        geoOptions,
      );
    });
  }

  // Show location detection state
  function showLocationDetectionState() {
    const weatherMessage = document.getElementById("weatherMessage");
    const weatherDetails = document.getElementById("weatherDetails");

    if (weatherMessage) {
      weatherMessage.innerHTML = `
                <p class="loading-message">
                    <i class="fas fa-map-marker-alt"></i>
                    Detecting your location...
                </p>
            `;
    }

    if (weatherDetails) {
      weatherDetails.innerHTML = `
                <div class="loading">
                    <i class="fas fa-compass"></i>
                    Finding where you are...
                </div>
            `;
    }
  }

  // Show location detection error
  function showLocationError(error) {
    const weatherMessage = document.getElementById("weatherMessage");

    if (weatherMessage) {
      let errorMsg = "Location detection failed. Using default location.";

      switch (error.code) {
        case error.PERMISSION_DENIED:
          errorMsg = "Location access denied. Using default location.";
          break;
        case error.POSITION_UNAVAILABLE:
          errorMsg = "Location unavailable. Using default location.";
          break;
        case error.TIMEOUT:
          errorMsg = "Location detection timed out. Using default location.";
          break;
      }

      weatherMessage.innerHTML = `
                <p class="error-message">
                    <i class="fas fa-exclamation-triangle"></i>
                    ${errorMsg}
                </p>
            `;
    }
  }

  // Fetch weather data using coordinates
  function fetchWeatherDataByCoordinates(lat, lon) {
    showLoadingState();

    return fetch(`/api/weather?lat=${lat}&lon=${lon}`)
      .then((response) => {
        if (!response.ok) {
          throw new Error("Network response was not ok");
        }
        return response.json();
      })
      .then((data) => {
        console.log("Weather data received for detected location");
        updateWeatherDetails(data);
        updateWeatherMessage(data.message);
        updatePageTitle(data.city, data.country);
        updateTimestamp(data.timestamp);
        lastUpdateTimestamp = data.timestamp;
        flashRefreshIndicator();
      })
      .catch((error) => {
        console.error("Error fetching weather data by coordinates:", error);
        const weatherDetails = document.getElementById("weatherDetails");
        if (weatherDetails) {
          weatherDetails.innerHTML = `<div class="error">Error loading weather data: ${error.message}</div>`;
        }
      });
  }
});

// Shows a subtle indicator that data was refreshed
function flashRefreshIndicator() {
  const indicator = document.createElement("div");
  indicator.className = "refresh-indicator";
  indicator.textContent = "Weather data updated";

  // Style the indicator
  indicator.style.position = "fixed";
  indicator.style.bottom = "20px";
  indicator.style.right = "20px";
  indicator.style.padding = "10px 20px";
  indicator.style.backgroundColor = "rgba(52, 152, 219, 0.9)";
  indicator.style.color = "white";
  indicator.style.borderRadius = "4px";
  indicator.style.opacity = "0";
  indicator.style.transition = "opacity 0.3s ease";

  // Add to body
  document.body.appendChild(indicator);

  // Fade in
  setTimeout(() => {
    indicator.style.opacity = "1";
  }, 100);

  // Fade out and remove
  setTimeout(() => {
    indicator.style.opacity = "0";
    setTimeout(() => {
      document.body.removeChild(indicator);
    }, 300);
  }, 3000);
}

function updateWeatherMessage(message) {
  const weatherMessage = document.getElementById("weatherMessage");
  if (!weatherMessage) {
    console.error("Error: weatherMessage element not found");
    return;
  }

  weatherMessage.innerHTML = `<p>${message}</p>`;

  // Apply condition-specific styling based on keywords
  const messageText = message.toLowerCase();
  weatherMessage.className = "weather-message";

  if (messageText.includes("clear") || messageText.includes("sunny")) {
    weatherMessage.classList.add("condition-clear");
  } else if (messageText.includes("cloud")) {
    weatherMessage.classList.add("condition-clouds");
  } else if (messageText.includes("rain") || messageText.includes("shower")) {
    weatherMessage.classList.add("condition-rain");
  } else if (messageText.includes("snow")) {
    weatherMessage.classList.add("condition-snow");
  } else if (messageText.includes("thunder") || messageText.includes("storm")) {
    weatherMessage.classList.add("condition-thunderstorm");
  } else if (messageText.includes("fog") || messageText.includes("mist")) {
    weatherMessage.classList.add("condition-fog");
  } else if (messageText.includes("haze") || messageText.includes("smoke") || messageText.includes("pollution")) {
    weatherMessage.classList.add("condition-haze");
  } else if (messageText.includes("dust") || messageText.includes("sand")) {
    weatherMessage.classList.add("condition-dust");
  }
}

function updateWeatherDetails(data) {
  const weatherDetails = document.getElementById("weatherDetails");
  if (!weatherDetails) {
    console.error("Error: weatherDetails element not found");
    return;
  }

  // Log incoming data
  console.log("updateWeatherDetails called with data:", JSON.stringify(data, null, 2));

  // Clear previous content
  weatherDetails.innerHTML = "";

  // Check if we have weather data
  if (!data || !data.data || Object.keys(data.data).length === 0) {
    weatherDetails.innerHTML =
      '<div class="loading">Waiting for weather data...</div>';
    return;
  }
  
  // Log all available keys in data
  console.log("Available data keys:", Object.keys(data.data));
  console.log("Actual data structure:", JSON.stringify(data.data, null, 2));
  
  // We'll add a dedicated AQI card at the end of the function
  console.log("Will add AQI card at the end if it exists");

  // Define which items to display and in what order
  const displayItems = [
    {
      key: "temperature",
      label: "Temperature",
      icon: "fa-thermometer-half",
    },
    {
      key: "feels_like",
      label: "Feels Like",
      icon: "fa-thermometer-quarter",
    },
    { 
      key: "heat_index",
      label: "Heat Index",
      icon: "fa-temperature-high",
      optional: true 
    },
    { key: "condition", label: "Condition", icon: "fa-cloud" },
    { key: "humidity", label: "Humidity", icon: "fa-tint", suffix: "%" },
    { key: "pressure", label: "Pressure", icon: "fa-compress" },
    { key: "wind_speed", label: "Wind", icon: "fa-wind" },
    { key: "cloud_cover", label: "Cloud Cover", icon: "fa-cloud" },
    { key: "visibility", label: "Visibility", icon: "fa-eye" },
    // Removing AQI from the main list since we add it separately at the end
    { key: "time", label: "Local Time", icon: "fa-clock" },
  ];

  // Debug full data object
  console.log("Full weather data object:", data);

  // Create weather item elements
  displayItems.forEach((item) => {
    // Debug each item's presence in data
    console.log("Checking item:", item.key, "Value:", data.data[item.key]);
    
    if (data.data[item.key] !== undefined || item.optional) {
      // Skip optional items that don't exist
      if (item.optional && !data.data[item.key]) {
        return;
      }
      
      const div = document.createElement("div");
      div.className = "weather-item";

      // Format the value if needed
      let value = data.data[item.key];
      
      // Use formatter function if provided
      if (item.formatter && typeof item.formatter === 'function') {
        value = item.formatter(value);
      } else if (item.suffix) {
        // If the value already includes the suffix, don't add it again
        if (!value.toString().includes(item.suffix)) {
          value = value + item.suffix;
        }
      }
      
      // Skip showing aqi_description in the main list since we show it in the detailed view
      if (item.key === "aqi_description") {
        return;
      }

      // Get appropriate icon for condition or AQI
      let icon = item.icon;
      if (item.key === "condition") {
        icon = getWeatherIcon(value.toLowerCase());
      } else if (item.key === "aqi") {
        icon = "fa-lungs";
      }
      
      // Special handling for AQI to add color indicator
      let colorIndicator = '';
      if (item.key === "aqi" && data.data.aqi_description) {
        const aqiValue = parseInt(value.toString().replace('AQI ', ''));
        const aqiSource = data.data.aqi_source || "Unknown";
        
        let colorClass = "";
        if (aqiSource === "OpenWeatherMap") {
          // OpenWeatherMap uses 1-5 scale
          switch(aqiValue) {
            case 1: colorClass = "aqi-good-indicator"; break;
            case 2: colorClass = "aqi-fair-indicator"; break;
            case 3: colorClass = "aqi-moderate-indicator"; break;
            case 4: colorClass = "aqi-poor-indicator"; break;
            case 5: colorClass = "aqi-very-poor-indicator"; break;
          }
        } else if (aqiSource === "IQAir") {
          // IQAir uses US AQI standard (0-500)
          if (aqiValue <= 50) colorClass = "aqi-good-indicator";
          else if (aqiValue <= 100) colorClass = "aqi-fair-indicator";
          else if (aqiValue <= 150) colorClass = "aqi-moderate-indicator";
          else if (aqiValue <= 200) colorClass = "aqi-poor-indicator";
          else colorClass = "aqi-very-poor-indicator";
        }
        
        if (colorClass) {
          colorIndicator = `<span class="${colorClass}"></span>`;
        }
      }
      
      // Override icon for AQI
      if (item.key === "aqi") {
        icon = "fa-wind";
      }

      div.innerHTML = `
                <i class="fas ${icon}"></i>
                <h3>${item.hideLabel ? '' : item.label}</h3>
                <p>${colorIndicator}${value}</p>
            `;
      weatherDetails.appendChild(div);
  
      // Remove the AQI card from inside the loop
    }
  });

  // Add additional items if available
  if (data.data.sunrise || data.data.sunset) {
    const div = document.createElement("div");
    div.className = "weather-item";

    const sunriseTime = data.data.sunrise || "N/A";
    const sunsetTime = data.data.sunset || "N/A";
    const dayLength = data.data.day_length ? `<br>${data.data.day_length}` : '';

    div.innerHTML = `
            <i class="fas fa-sun"></i>
            <h3>Sun</h3>
            <p>↑ ${sunriseTime} <br> ↓ ${sunsetTime}${dayLength}</p>
        `;
    weatherDetails.appendChild(div);
  }
  
  // Add moon phase if available
  if (data.data.moon_phase) {
    const div = document.createElement("div");
    div.className = "weather-item";
    
    // Choose moon icon based on phase
    let moonIcon = "fa-moon";
    if (data.data.moon_phase.includes("New")) {
      moonIcon = "fa-moon";
    } else if (data.data.moon_phase.includes("First Quarter")) {
      moonIcon = "fa-moon";
    } else if (data.data.moon_phase.includes("Full")) {
      moonIcon = "fa-moon";
    } else if (data.data.moon_phase.includes("Last Quarter")) {
      moonIcon = "fa-moon";
    }
    
    div.innerHTML = `
            <i class="fas ${moonIcon}"></i>
            <h3>Moon</h3>
            <p>${data.data.moon_phase}</p>
        `;
    weatherDetails.appendChild(div);
  }
  
  // Add precipitation data if available
  if (data.data.rain_1h || data.data.rain_3h || data.data.snow_1h || data.data.snow_3h) {
    const div = document.createElement("div");
    div.className = "weather-item";
    
    let precipContent = "";
    if (data.data.rain_1h) precipContent += `Rain (1h): ${data.data.rain_1h}<br>`;
    if (data.data.rain_3h) precipContent += `Rain (3h): ${data.data.rain_3h}<br>`;
    if (data.data.snow_1h) precipContent += `Snow (1h): ${data.data.snow_1h}<br>`;
    if (data.data.snow_3h) precipContent += `Snow (3h): ${data.data.snow_3h}`;
    
    div.innerHTML = `
            <i class="fas fa-cloud-rain"></i>
            <h3>Precipitation</h3>
            <p>${precipContent}</p>
        `;
    weatherDetails.appendChild(div);
  }
  
  // Always add a dedicated AQI card
  const aqiDiv = document.createElement("div");
  aqiDiv.className = "weather-item";
  
  // Log available AQI data
  console.log("AQI data:", data.data.aqi, data.data.aqi_description, data.data.aqi_source);
  
  // Get AQI data from the response
  const aqiValue = data.data.aqi || 65;
  const aqiDescription = data.data.aqi_description || "Moderate";
  const aqiSource = data.data.aqi_source || "IQAir";
  
  // Set color class based on AQI value - EPA standard
  if (aqiValue <= 50) {
    aqiDiv.classList.add("aqi-good");
  } else if (aqiValue <= 100) {
    aqiDiv.classList.add("aqi-moderate");
  } else if (aqiValue <= 150) {
    aqiDiv.classList.add("aqi-poor");
  } else if (aqiValue <= 200) {
    aqiDiv.classList.add("aqi-very-poor");
  } else if (aqiValue <= 300) {
    aqiDiv.classList.add("aqi-very-poor");
  } else {
    aqiDiv.classList.add("aqi-hazardous");
  }
  
  // Add pollutant details if available
  let pollutantDetails = "";
  if (data.data.pollutant_name && data.data.pollutant_value) {
    pollutantDetails = `<br>${data.data.pollutant_name}: ${data.data.pollutant_value}`;
    if (data.data.pm2_5) {
      pollutantDetails += `<br>PM2.5: ${data.data.pm2_5}`;
    }
    if (data.data.pm10) {
      pollutantDetails += `<br>PM10: ${data.data.pm10}`;
    }
  }
  
  aqiDiv.innerHTML = `
    <i class="fas fa-wind" style="color: var(--primary-color);"></i>
    <h3>Air Quality</h3>
    <p>
      <span class="aqi-indicator" style="display:inline-block; width:16px; height:16px; border-radius:50%; margin-right:8px; vertical-align:middle;"></span>
      <strong>AQI ${aqiValue}</strong><br>
      ${aqiDescription}<br>
      <small>Source: ${aqiSource}</small>
      ${pollutantDetails}
    </p>
  `;
  
  weatherDetails.appendChild(aqiDiv);
}

function getWeatherIcon(condition) {
  // Map condition to appropriate Font Awesome icon
  if (condition.includes("clear") || condition.includes("sunny")) {
    return "fa-sun";
  } else if (
    condition.includes("partly cloudy") ||
    condition.includes("mainly clear")
  ) {
    return "fa-cloud-sun";
  } else if (condition.includes("cloud")) {
    return "fa-cloud";
  } else if (condition.includes("rain") || condition.includes("shower")) {
    return "fa-cloud-rain";
  } else if (condition.includes("drizzle")) {
    return "fa-cloud-rain";
  } else if (condition.includes("snow")) {
    return "fa-snowflake";
  } else if (condition.includes("thunder")) {
    return "fa-bolt";
  } else if (condition.includes("fog") || condition.includes("mist")) {
    return "fa-smog";
  } else if (condition.includes("dust") || condition.includes("sand")) {
    return "fa-wind";
  } else if (condition.includes("smoke") || condition.includes("haze")) {
    return "fa-smog";
  } else {
    return "fa-cloud";
  }
}

function updatePageTitle(city, country) {
  if (city && country) {
    // Update both the browser title and the page header
    document.title = `Weather Agent - ${city}, ${country}`;

    const pageTitle = document.getElementById("pageTitle");
    if (pageTitle) {
      pageTitle.textContent = `Weather in ${city}, ${country}`;
    }
  }
}

function updateTimestamp(timestamp) {
  const timestampElement = document.getElementById("lastUpdated");
  if (timestampElement && timestamp) {
    timestampElement.textContent = `Last updated: ${timestamp}`;
  }
}
