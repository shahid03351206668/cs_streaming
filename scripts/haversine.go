package main

import (
	"fmt"
	"math"
)

const earthRadiusMeters = 6371000 // Earth's radius in meters

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad := lat1 * math.Pi / 180.0
	lon1Rad := lon1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	lon2Rad := lon2 * math.Pi / 180.0

	dLon := lon2Rad - lon1Rad
	dLat := lat2Rad - lat1Rad

	// Haversine formula
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Asin(math.Sqrt(a))

	distance := earthRadiusMeters * c
	return distance
}

func isWithinRadiusManual(
	centerLat, centerLon, pointLat, pointLon float64,
	radiusMeters float64,
) bool {
	distance := haversineDistance(centerLat, centerLon, pointLat, pointLon)
	return distance <= radiusMeters
}

func main() {
	newYorkLat := 40.7128
	newYorkLon := -74.0060

	sanDiegoLat := 32.7157
	sanDiegoLon := -117.1611
	radius := 100_0000.0 // 1000 km in meters

	within := isWithinRadiusManual(newYorkLat, newYorkLon, sanDiegoLat, sanDiegoLon, radius)
	fmt.Printf("Is San Diego within %f meters of New York? %t\n", radius, within)
}
