#!/bin/bash

# Read API key from .env file
if [ -f .env ]; then
  IQAIR_API_KEY=$(grep "IQAIR_API_KEY" .env | cut -d '=' -f2)
fi

# Use environment variable if set, otherwise use the one from .env
API_KEY=${IQAIR_API_KEY:-$IQAIR_API_KEY}

if [ -z "$API_KEY" ]; then
  echo "Error: IQAir API key not found. Please set IQAIR_API_KEY environment variable or add it to .env file."
  exit 1
fi

echo "Testing IQAir API with key: ${API_KEY:0:4}... (length: ${#API_KEY})"
echo

# Test coordinates for New York City
LAT=40.7128
LON=-74.0060

echo "=== Testing nearest_city endpoint ==="
echo "API URL: https://api.airvisual.com/v2/nearest_city?lat=$LAT&lon=$LON&key=[REDACTED]"
echo "Sending request to IQAir API..."
echo

RESPONSE=$(curl -s "https://api.airvisual.com/v2/nearest_city?lat=$LAT&lon=$LON&key=$API_KEY")
echo "Response:"
echo "$RESPONSE" | python3 -m json.tool

# Check if the response was successful
if echo "$RESPONSE" | grep -q '"status":"success"'; then
  echo
  CITY=$(echo "$RESPONSE" | grep -o '"city":"[^"]*"' | cut -d':' -f2 | tr -d '"')
  STATE=$(echo "$RESPONSE" | grep -o '"state":"[^"]*"' | cut -d':' -f2 | tr -d '"')
  COUNTRY=$(echo "$RESPONSE" | grep -o '"country":"[^"]*"' | cut -d':' -f2 | tr -d '"')
  AQI=$(echo "$RESPONSE" | grep -o '"aqius":[0-9]*' | cut -d':' -f2)
  TEMP=$(echo "$RESPONSE" | grep -o '"tp":[0-9.]*' | cut -d':' -f2)
  
  echo "Successfully retrieved data for $CITY, $STATE, $COUNTRY"
  echo "Current AQI (US): $AQI"
  echo "Temperature: ${TEMP}°C"
else
  echo
  echo "API request failed"
  echo "Response status: $(echo "$RESPONSE" | grep -o '"status":"[^"]*"' | cut -d':' -f2 | tr -d '"')"
fi

echo
echo "=== Testing specific states list ==="
echo "API URL: https://api.airvisual.com/v2/states?country=USA&key=[REDACTED]"
echo "Sending request to IQAir API..."
echo

RESPONSE=$(curl -s "https://api.airvisual.com/v2/states?country=USA&key=$API_KEY")
echo "Response:"
echo "$RESPONSE" | python3 -m json.tool

echo
echo "=== Testing specific cities list ==="
echo "API URL: https://api.airvisual.com/v2/cities?state=New%20York&country=USA&key=[REDACTED]"
echo "Sending request to IQAir API..."
echo

RESPONSE=$(curl -s "https://api.airvisual.com/v2/cities?state=New%20York&country=USA&key=$API_KEY")
echo "Response:"
echo "$RESPONSE" | python3 -m json.tool

# If city list was successful, test a specific city
if echo "$RESPONSE" | grep -q '"status":"success"'; then
  FIRST_CITY=$(echo "$RESPONSE" | grep -o '"city":"[^"]*"' | head -1 | cut -d':' -f2 | tr -d '"')
  
  if [ -n "$FIRST_CITY" ]; then
    FIRST_CITY_ENCODED=$(echo "$FIRST_CITY" | sed 's/ /%20/g')
    
    echo
    echo "=== Testing specific city: $FIRST_CITY ==="
    echo "API URL: https://api.airvisual.com/v2/city?city=$FIRST_CITY_ENCODED&state=New%20York&country=USA&key=[REDACTED]"
    echo "Sending request to IQAir API..."
    echo
    
    RESPONSE=$(curl -s "https://api.airvisual.com/v2/city?city=$FIRST_CITY_ENCODED&state=New%20York&country=USA&key=$API_KEY")
    echo "Response:"
    echo "$RESPONSE" | python3 -m json.tool
  fi
fi

echo
echo "Tests completed."