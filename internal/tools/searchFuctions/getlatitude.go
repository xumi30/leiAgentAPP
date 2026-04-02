package searchFunctions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// GeocodingTool implements the Tool interface for converting location names to coordinates
type GeocodingTool struct{}

// NewGeocodingTool creates a new GeocodingTool instance
func NewGeocodingTool() *GeocodingTool {
	return &GeocodingTool{}
}

// Name returns the name of the tool
func (t *GeocodingTool) Name() string {
	return "geocoding"
}

// Description returns a description of what the tool does
func (t *GeocodingTool) Description() string {
	return "Converts a location name (city, address, landmark) into latitude and longitude coordinates. Input should be a place name like 'Beijing', 'New York', or 'Eiffel Tower'. Returns the coordinates and additional location details."
}

// Parameters returns the parameters that the tool accepts
func (t *GeocodingTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"location": map[string]interface{}{
				"type":        "string",
				"description": "The location name to geocode. Must use 'location' as the parameter name. Examples: 'Beijing', 'Times Square', 'Tokyo Tower'",
			},
		},
		"required": []string{"location"},
	}
}

// Run executes the tool with the given input
func (t *GeocodingTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute executes the tool with the given arguments
func (t *GeocodingTool) Execute(ctx context.Context, args string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid input format: %v", err)
	}

	var location string
	//尝试只要有一个键值对就取出value作为location
	for _, value := range params {
		location = fmt.Sprintf("%v", value)
		break
	}
	if location == "" {
		return "", fmt.Errorf("location parameter is required")
	}
	// // Get location parameter
	// location, ok := params["location"].(string)
	// if !ok || location == "" {
	// 	//尝试只要有一个键值对就取出value作为location
	// 	for _, value := range params {
	// 		location = fmt.Sprintf("%v", value)
	// 		break
	// 	}
	// 	return "", fmt.Errorf("location parameter is required")
	// }

	// Build API URL
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1",
		url.QueryEscape(location))

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Add required User-Agent header for Nominatim API
	req.Header.Set("User-Agent", "LLMWeatherTool/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch geocoding data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("geocoding API returned status: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Parse JSON response
	var results []map[string]interface{}
	if err := json.Unmarshal(body, &results); err != nil {
		return "", fmt.Errorf("failed to parse geocoding response: %v", err)
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no location found for '%s'", location)
	}

	// Extract coordinates from the first result
	result := results[0]
	lat, ok := result["lat"].(string)
	if !ok {
		return "", fmt.Errorf("invalid latitude in response")
	}
	lon, ok := result["lon"].(string)
	if !ok {
		return "", fmt.Errorf("invalid longitude in response")
	}

	// Format the response
	displayName, _ := result["display_name"].(string)
	return fmt.Sprintf("Location: %s\nCoordinates: %s, %s", displayName, lat, lon), nil
}
