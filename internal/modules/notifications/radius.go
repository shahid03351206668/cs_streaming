package notifications

import (
	"errors"
	"strings"

	"tasksy/models"

	"gorm.io/gorm"
)

const MaxJobRadiusKM = 500.0

func sameCity(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func hasCentre(p *models.NotificationPreference) bool {
	return p.Latitude != nil && p.Longitude != nil
}

// RadiusActive reports whether new jobs are matched by distance from the
// user's city centre, rather than by the city name alone.
func RadiusActive(p *models.NotificationPreference) bool {
	return hasCentre(p) && p.JobRadiusKM > 0
}

// matchJobLocation decides whether a new job falls inside what the user chose:
// within the radius of their city centre, or in their city by name when no
// radius can be applied. The distance is returned when it is known.
func matchJobLocation(p *models.NotificationPreference, loc *models.JobPostLocation) (bool, *float64) {
	jobHasPosition := loc.Latitude != 0 || loc.Longitude != 0

	var distance *float64
	if hasCentre(p) && jobHasPosition {
		d := haversineKM(*p.Latitude, *p.Longitude, loc.Latitude, loc.Longitude)
		distance = &d
	}

	switch {
	case RadiusActive(p) && distance != nil:
		return *distance <= p.JobRadiusKM, distance
	case strings.TrimSpace(p.City) != "":
		return sameCity(p.City, loc.City), distance
	}
	return true, distance
}

func validateLocationInput(lat, lng, radius *float64) error {
	if lat != nil && (*lat < -90 || *lat > 90) {
		return errors.New("latitude must be between -90 and 90")
	}
	if lng != nil && (*lng < -180 || *lng > 180) {
		return errors.New("longitude must be between -180 and 180")
	}
	if radius != nil && (*radius < 0 || *radius > MaxJobRadiusKM) {
		return errors.New("job_radius_km must be between 0 and 500")
	}
	return nil
}

// cityCentre estimates a city's centre from where jobs in that city have been
// posted, for apps that send a city name without coordinates.
func cityCentre(db *gorm.DB, city string) (lat, lng float64, ok bool) {
	var row struct {
		Lat, Lng float64
		N        int64
	}
	err := db.Model(&models.JobPostLocation{}).
		Where("LOWER(TRIM(city)) = LOWER(TRIM(?)) AND (latitude <> 0 OR longitude <> 0)", city).
		Select("COALESCE(AVG(latitude), 0) AS lat, COALESCE(AVG(longitude), 0) AS lng, COUNT(*) AS n").
		Scan(&row).Error
	if err != nil || row.N == 0 {
		return 0, 0, false
	}
	return row.Lat, row.Lng, true
}
