document.addEventListener("DOMContentLoaded", function () {
    // Check if all required elements exist before proceeding
    const weatherDetailsElement = document.getElementById("weatherDetails");
    const weatherMessageElement = document.getElementById("weatherMessage");

    if (!weatherDetailsElement) {
        console.error(
            'Error: Element with ID "weatherDetails" not found in the document'
        );
        return;
    }

    if (!weatherMessageElement) {
        console.error(
            'Error: Element with ID "weatherMessage" not found in the document'
        );
        return;
    }

    // Track last update timestamp to detect changes
    let lastUpdateTimestamp = "";

    // Fetch weather data when page loads
    fetchWeatherData().then(() => {
        // Set up automatic refresh every 15 seconds
        // This checks more frequently but only updates the UI when there's new data
        setInterval(() => {
            console.log("Checking for new weather data...");
            fetchWeatherData(true);
        }, 15000); // Check every 15 seconds
    });

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

            // Force refresh (don't check timestamp)
            fetchWeatherData(false).finally(() => {
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
                    const weatherDetails =
                        document.getElementById("weatherDetails");
                    if (weatherDetails) {
                        weatherDetails.innerHTML = `<div class="error">Error loading weather data: ${error.message}</div>`;
                    }
                }
            });
    }

    // Add this to the global scope so our interval can use it
    window.fetchWeatherData = fetchWeatherData;
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
    } else if (
        messageText.includes("thunder") ||
        messageText.includes("storm")
    ) {
        weatherMessage.classList.add("condition-thunderstorm");
    } else if (messageText.includes("fog") || messageText.includes("mist")) {
        weatherMessage.classList.add("condition-fog");
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
        { key: "condition", label: "Condition", icon: "fa-cloud" },
        { key: "humidity", label: "Humidity", icon: "fa-tint", suffix: "%" },
        { key: "wind_speed", label: "Wind", icon: "fa-wind" },
        { key: "time", label: "Local Time", icon: "fa-clock" },
    ];

    // Create weather item elements
    displayItems.forEach((item) => {
        if (data.data[item.key]) {
            const div = document.createElement("div");
            div.className = "weather-item";

            // Format the value if needed
            let value = data.data[item.key];
            if (item.suffix) {
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

        div.innerHTML = `
            <i class="fas fa-sun"></i>
            <h3>Sun</h3>
            <p>↑ ${sunriseTime} <br> ↓ ${sunsetTime}</p>
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
    } else {
        return "fa-cloud";
    }
}

function updatePageTitle(city, country) {
    if (city && country) {
        document.title = `Weather in ${city}, ${country}`;
    }
}

function updateTimestamp(timestamp) {
    const timestampElement = document.querySelector(".timestamp");
    if (timestampElement && timestamp) {
        timestampElement.textContent = `Last updated: ${timestamp}`;
    }
}
