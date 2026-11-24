package notifications

import (
	"math"
	"testing"

	"tasksy/models"
)

func f(v float64) *float64 { return &v }

var karachi = struct{ lat, lng float64 }{24.8607, 67.0011}

func pref(city string, centre bool, radius float64) *models.NotificationPreference {
	p := &models.NotificationPreference{City: city, JobRadiusKM: radius}
	if centre {
		p.Latitude, p.Longitude = f(karachi.lat), f(karachi.lng)
	}
	return p
}

func TestHaversineKnownDistances(t *testing.T) {
	for _, c := range []struct {
		name                         string
		lat1, lng1, lat2, lng2, want float64
	}{
		{"Karachi to Lahore", 24.8607, 67.0011, 31.5204, 74.3587, 1030},
		{"Karachi to Hyderabad", 24.8607, 67.0011, 25.3960, 68.3578, 149},
		{"same point", 24.8607, 67.0011, 24.8607, 67.0011, 0},
	} {
		got := haversineKM(c.lat1, c.lng1, c.lat2, c.lng2)
		if math.Abs(got-c.want) > c.want*0.02+0.1 {
			t.Errorf("%s: %.1f km, want about %.0f", c.name, got, c.want)
		}
	}
}

func TestMatchJobLocation(t *testing.T) {
	clifton := &models.JobPostLocation{City: "Karachi", Latitude: 24.8138, Longitude: 67.0300}
	gulshan := &models.JobPostLocation{City: "Gulshan-e-Iqbal", Latitude: 24.9215, Longitude: 67.0924}
	hyderabad := &models.JobPostLocation{City: "Hyderabad", Latitude: 25.3960, Longitude: 68.3578}
	lahore := &models.JobPostLocation{City: "Lahore", Latitude: 31.5204, Longitude: 74.3587}
	noCoords := &models.JobPostLocation{City: "Karachi"}

	cases := []struct {
		name string
		p    *models.NotificationPreference
		job  *models.JobPostLocation
		want bool
	}{
		{"inside the radius", pref("Karachi", true, 50), clifton, true},
		{"a different city name inside the radius still matches", pref("Karachi", true, 50), gulshan, true},
		{"a neighbouring city inside a wide radius matches", pref("Karachi", true, 200), hyderabad, true},
		{"outside the radius", pref("Karachi", true, 50), hyderabad, false},
		{"a far city never matches", pref("Karachi", true, 200), lahore, false},
		{"just outside a small radius", pref("Karachi", true, 5), clifton, false},
		{"just inside a larger radius", pref("Karachi", true, 10), clifton, true},
		{"radius 0 means the city name only: same city", pref("Karachi", true, 0), clifton, true},
		{"radius 0 means the city name only: other label", pref("Karachi", true, 0), gulshan, false},
		{"city by name only, no centre: same city", pref("karachi ", false, 50), clifton, true},
		{"city by name only, no centre: other city", pref("Karachi", false, 50), lahore, false},
		{"a job with no coordinates falls back to the city name: same", pref("Karachi", true, 5), noCoords, true},
		{"a job with no coordinates falls back to the city name: other", pref("Lahore", true, 50), noCoords, false},
		{"no city and no centre matches every job", pref("", false, 50), lahore, true},
		{"a centre without a city still applies the radius", pref("", true, 50), hyderabad, false},
		{"a centre without a city, inside", pref("", true, 50), gulshan, true},
	}
	for _, c := range cases {
		got, _ := matchJobLocation(c.p, c.job)
		if got != c.want {
			t.Errorf("%s: matched=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestDistanceIsReportedOnlyWhenKnown(t *testing.T) {
	_, d := matchJobLocation(pref("Karachi", true, 50), &models.JobPostLocation{City: "Karachi", Latitude: 24.8138, Longitude: 67.0300})
	if d == nil || *d < 5 || *d > 7 {
		t.Errorf("distance %v, want about 6 km", d)
	}
	if _, d := matchJobLocation(pref("Karachi", false, 50), &models.JobPostLocation{City: "Karachi", Latitude: 24.8, Longitude: 67}); d != nil {
		t.Errorf("a distance was reported with no city centre")
	}
	if _, d := matchJobLocation(pref("Karachi", true, 50), &models.JobPostLocation{City: "Karachi"}); d != nil {
		t.Errorf("a distance was reported for a job with no position")
	}
}

func TestValidateLocationInput(t *testing.T) {
	bad := []struct {
		name          string
		lat, lng, rad *float64
	}{
		{"latitude too high", f(91), nil, nil}, {"latitude too low", f(-91), nil, nil},
		{"longitude too high", nil, f(181), nil}, {"longitude too low", nil, f(-181), nil},
		{"negative radius", nil, nil, f(-1)}, {"radius above the maximum", nil, nil, f(501)},
	}
	for _, c := range bad {
		if validateLocationInput(c.lat, c.lng, c.rad) == nil {
			t.Errorf("%s was accepted", c.name)
		}
	}
	for _, c := range []struct{ lat, lng, rad *float64 }{{f(90), f(180), f(500)}, {f(-90), f(-180), f(0)}, {nil, nil, nil}} {
		if err := validateLocationInput(c.lat, c.lng, c.rad); err != nil {
			t.Errorf("valid input refused: %v", err)
		}
	}
}

func TestRadiusActive(t *testing.T) {
	if RadiusActive(pref("Karachi", false, 50)) || RadiusActive(pref("Karachi", true, 0)) {
		t.Error("radius reported active without a centre or with radius 0")
	}
	if !RadiusActive(pref("Karachi", true, 50)) {
		t.Error("radius not active with a centre and a radius")
	}
}
