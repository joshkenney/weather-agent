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
        return response.json();
      })
      .then((data) => {
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

  // Clear previous content
  weatherDetails.innerHTML = "";

  // Check if we have weather data
  if (!data.data || Object.keys(data.data).length === 0) {
    weatherDetails.innerHTML =
      '<div class="loading">Waiting for weather data...</div>';
    return;
  }

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
    { 
      key: "aqi", 
      label: "Air Quality", 
      icon: "fa-wind", 
      optional: true,
      formatter: function(value) {
        return `AQI ${value}`;
      }
    },
    { key: "time", label: "Local Time", icon: "fa-clock" },
  ];

  // Create weather item elements
  displayItems.forEach((item) => {
    if (data.data[item.key] || item.optional) {
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

      // Get appropriate icon for condition
      let icon = item.icon;
      if (item.key === "condition") {
        icon = getWeatherIcon(value.toLowerCase());
      }

      div.innerHTML = `
                <i class="fas ${icon}"></i>
                <h3>${item.label}</h3>
                <p>${value}</p>
            `;
      weatherDetails.appendChild(div);
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
  
  // Add detailed AQI information if available
  if (data.data.aqi && data.data.aqi_description) {
    const div = document.createElement("div");
    div.className = "weather-item";
    
    // Style based on AQI value
    let aqiClass = "";
    switch(parseInt(data.data.aqi)) {
      case 1: aqiClass = "aqi-good"; break;
      case 2: aqiClass = "aqi-fair"; break;
      case 3: aqiClass = "aqi-moderate"; break;
      case 4: aqiClass = "aqi-poor"; break;
      case 5: aqiClass = "aqi-very-poor"; break;
    }
    
    if (aqiClass) {
      div.classList.add(aqiClass);
    }
    
    // Create pollutant details if available
    let pollutantDetails = "";
    if (data.data.pm2_5) pollutantDetails += `PM2.5: ${data.data.pm2_5}<br>`;
    if (data.data.pm10) pollutantDetails += `PM10: ${data.data.pm10}<br>`;
    if (data.data.o3) pollutantDetails += `O₃: ${data.data.o3}<br>`;
    if (data.data.no2) pollutantDetails += `NO₂: ${data.data.no2}`;
    
    let aqiContent = data.data.aqi_description;
    if (pollutantDetails) {
      aqiContent += `<br><details>
        <summary>Pollutant Details</summary>
        <p>${pollutantDetails}</p>
      </details>`;
    }
    
    div.innerHTML = `
            <i class="fas fa-wind"></i>
            <h3>Air Quality</h3>
            <p>${aqiContent}</p>
        `;
    weatherDetails.appendChild(div);
  }
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
