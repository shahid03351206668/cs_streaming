package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	karachiLat, karachiLng = 24.8607, 67.0011
	allJobs                = "clifton hyderabad gulshan lahore nocoords"
)

func (s *suite) setup() {
	s.r.Section("Setup")
	s.r.MustCheck("register the job poster and nine users", func() (string, error) {
		for _, n := range []string{"poster", "alice", "bob", "carol", "dave", "erin", "frank", "gina", "greg", "ivy"} {
			if _, err := s.register(n); err != nil {
				return "", fmt.Errorf("%s: %w", n, err)
			}
		}
		return "10 users", nil
	})
	s.r.MustCheck("a job category exists", func() (string, error) {
		status, resp, err := s.api.JSON("GET", "/api/v1/category/list", "", nil)
		if err != nil {
			return "", err
		}
		list, _ := resp["data"].([]any)
		if status != 200 || len(list) == 0 {
			return "", fmt.Errorf("no categories (status %d); create one in the admin panel first", status)
		}
		first, _ := list[0].(map[string]any)
		s.cat = str(first, "id")
		return s.cat, nil
	})
}

func (s *suite) validation() {
	s.r.Section("Preferences API validation")
	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"latitude above 90", map[string]any{"latitude": 91.0, "longitude": 10.0}},
		{"latitude below -90", map[string]any{"latitude": -91.0, "longitude": 10.0}},
		{"longitude above 180", map[string]any{"latitude": 10.0, "longitude": 181.0}},
		{"longitude below -180", map[string]any{"latitude": 10.0, "longitude": -181.0}},
		{"negative radius", map[string]any{"job_radius_km": -1.0}},
		{"radius above 500 km", map[string]any{"job_radius_km": 501.0}},
		{"latitude without longitude", map[string]any{"latitude": 10.0}},
		{"longitude without latitude", map[string]any{"longitude": 10.0}},
	} {
		c := c
		s.r.Check(c.name+" is refused (400)", func() (string, error) {
			status, resp, err := s.setPrefs("alice", c.body)
			if err != nil {
				return "", err
			}
			if status != 400 {
				return "", fmt.Errorf("status %d: %v", status, resp)
			}
			return errMsg(resp), nil
		})
	}
	s.r.Check("a refused update leaves the saved preferences unchanged", func() (string, error) {
		status, resp, err := s.api.JSON("GET", prefsPath, s.users["alice"].token, nil)
		if err != nil || status != 200 {
			return "", fmt.Errorf("status %d %v", status, err)
		}
		if r, _ := num(resp, "data", "job_radius_km"); r != 50 {
			return "", fmt.Errorf("radius is %v, want the default 50", r)
		}
		return "radius still 50", nil
	})
	s.r.Check("the limits themselves are accepted", func() (string, error) {
		status, resp, err := s.setPrefs("ivy", map[string]any{"latitude": 90.0, "longitude": 180.0, "job_radius_km": 500.0})
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		return "lat 90, lng 180, radius 500", nil
	})
	s.r.Check("preferences require a login", func() (string, error) {
		status, _, err := s.api.JSON("GET", prefsPath, "", nil)
		if err != nil {
			return "", err
		}
		return fmt.Sprint(status), expect(status, 401)
	})
}

func expect(got, want int) error {
	if got != want {
		return fmt.Errorf("status %d, want %d", got, want)
	}
	return nil
}

func (s *suite) matching() {
	s.r.Section("Who is notified, by city and radius")

	s.r.MustCheck("a job in Karachi is posted first, so a city name alone can be turned into a centre", func() (string, error) {
		return "seeded", s.postJob("poster", "seed", "Karachi", karachiLat, karachiLng)
	})

	centre := func(radius float64) map[string]any {
		return map[string]any{"city": "Karachi", "latitude": karachiLat, "longitude": karachiLng, "job_radius_km": radius}
	}
	prefs := []struct {
		user string
		body map[string]any
		desc string
	}{
		{"alice", centre(50), "Karachi, 50 km"},
		{"bob", centre(5), "Karachi, 5 km"},
		{"carol", map[string]any{"city": "Lahore", "latitude": 31.5204, "longitude": 74.3587, "job_radius_km": 50.0}, "Lahore, 50 km"},
		{"dave", map[string]any{"city": "Karachi", "job_radius_km": 30.0}, "Karachi by name only, 30 km (centre looked up)"},
		{"erin", func() map[string]any { m := centre(50); m["enable_new_jobs"] = false; return m }(), "Karachi, 50 km, new-job notifications off"},
		{"frank", map[string]any{"city": "", "job_radius_km": 50.0}, "no city: every job"},
		{"gina", centre(0), "Karachi, radius 0: city name only"},
		{"greg", centre(200), "Karachi, 200 km"},
		{"poster", centre(50), "Karachi, 50 km, and the one posting"},
	}
	for _, p := range prefs {
		p := p
		s.r.MustCheck(p.user+" chooses "+p.desc, func() (string, error) {
			status, resp, err := s.setPrefs(p.user, p.body)
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
			}
			active := dig(resp, "meta", "radius_active")
			return fmt.Sprintf("radius_active=%v", active), nil
		})
	}

	s.r.Check("a city given by name only gets a centre, so the radius is active", func() (string, error) {
		_, resp, err := s.api.JSON("GET", prefsPath, s.users["dave"].token, nil)
		if err != nil {
			return "", err
		}
		lat, ok := num(resp, "data", "latitude")
		if !ok || lat < 24 || lat > 26 {
			return "", fmt.Errorf("latitude %v, want a point in Karachi", dig(resp, "data", "latitude"))
		}
		if dig(resp, "meta", "radius_active") != true {
			return "", fmt.Errorf("radius_active is not true")
		}
		return fmt.Sprintf("centre %.2f, radius active", lat), nil
	})

	jobs := []struct {
		key, city string
		lat, lng  float64
	}{
		{"clifton", "Karachi", 24.8138, 67.0300},
		{"hyderabad", "Hyderabad", 25.3960, 68.3578},
		{"gulshan", "Gulshan-e-Iqbal", 24.9215, 67.0924},
		{"lahore", "Lahore", 31.5204, 74.3587},
		{"nocoords", "Karachi", 0, 0},
	}
	for _, j := range jobs {
		j := j
		s.r.MustCheck("post a job: "+j.key, func() (string, error) {
			return fmt.Sprintf("%s (%.4f, %.4f)", j.city, j.lat, j.lng), s.postJob("poster", j.key, j.city, j.lat, j.lng)
		})
	}

	// What each user should have been notified about. clifton is about 6 km
	// from the Karachi centre, gulshan about 10 km, hyderabad about 149 km.
	want := map[string]string{
		"alice":  "clifton gulshan nocoords seed",
		"bob":    "nocoords seed",
		"carol":  "lahore",
		"dave":   "clifton gulshan nocoords seed",
		"erin":   "",
		"frank":  allJobs + " seed",
		"gina":   "clifton nocoords seed",
		"greg":   "clifton gulshan hyderabad nocoords seed",
		"poster": "",
	}
	// seed is posted before anyone chose a preference, so nobody can have it.
	for u, w := range want {
		want[u] = strings.TrimSpace(strings.ReplaceAll(" "+w+" ", " seed ", " "))
	}
	// alice's preferences row already existed (the validation checks read it,
	// which creates the default: no city, so every job), so she did get seed.
	want["alice"] += " seed"

	time.Sleep(4 * time.Second)
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		n := n
		s.r.Check(n+" is notified about exactly: ["+want[n]+"]", func() (string, error) {
			var got map[string]string
			deadline := time.Now().Add(15 * time.Second)
			for {
				var err error
				if got, err = s.notified(n); err != nil {
					return "", err
				}
				if same(keys(got), strings.Fields(want[n])) || time.Now().After(deadline) {
					break
				}
				time.Sleep(time.Second)
			}
			if !same(keys(got), strings.Fields(want[n])) {
				return "", fmt.Errorf("notified about [%s], want [%s]", strings.Join(keys(got), " "), want[n])
			}
			return strings.Join(keys(got), " "), nil
		})
	}

	s.r.Check("a notification inside a radius says how far away the job is", func() (string, error) {
		got, err := s.notified("alice")
		if err != nil {
			return "", err
		}
		body := got["clifton"]
		if !strings.Contains(body, "km away") || !strings.Contains(body, s.jobs["clifton"].title) {
			return "", fmt.Errorf("body %q", body)
		}
		return body, nil
	})
	s.r.Check("a job with no position is announced without a distance", func() (string, error) {
		got, err := s.notified("alice")
		if err != nil {
			return "", err
		}
		if strings.Contains(got["nocoords"], "km") {
			return "", fmt.Errorf("body %q", got["nocoords"])
		}
		return got["nocoords"], nil
	})
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func same(a, b []string) bool {
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, " ") == strings.Join(y, " ")
}

func (s *suite) cityChange() {
	s.r.Section("Changing the city doesn't keep the old centre")
	s.r.Check("ivy moves from Karachi to Lahore by name; the centre becomes Lahore's, not Karachi's", func() (string, error) {
		if status, resp, err := s.setPrefs("ivy", map[string]any{"city": "Karachi", "latitude": karachiLat, "longitude": karachiLng, "job_radius_km": 50.0}); err != nil || status != 200 {
			return "", fmt.Errorf("setup: %d %v %s", status, err, errMsg(resp))
		}
		status, resp, err := s.setPrefs("ivy", map[string]any{"city": "Lahore"})
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		lat, ok := num(resp, "data", "latitude")
		if ok && lat < 30 {
			return "", fmt.Errorf("the radius would be measured from latitude %.2f (Karachi), not Lahore", lat)
		}
		return fmt.Sprintf("centre latitude %v", dig(resp, "data", "latitude")), nil
	})
	s.r.Check("a city with no jobs yet has no centre, so matching falls back to the city name", func() (string, error) {
		status, resp, err := s.setPrefs("ivy", map[string]any{"city": fmt.Sprintf("Nowhere%d", s.run)})
		if err != nil || status != 200 {
			return "", fmt.Errorf("status %d %v", status, err)
		}
		if dig(resp, "data", "latitude") != nil || dig(resp, "meta", "radius_active") != false {
			return "", fmt.Errorf("latitude=%v radius_active=%v", dig(resp, "data", "latitude"), dig(resp, "meta", "radius_active"))
		}
		return "no centre, radius_active=false", nil
	})
	s.r.Check("clearing the city clears the centre too, so every job matches again", func() (string, error) {
		status, resp, err := s.setPrefs("ivy", map[string]any{"city": ""})
		if err != nil || status != 200 {
			return "", fmt.Errorf("status %d %v", status, err)
		}
		if dig(resp, "data", "latitude") != nil {
			return "", fmt.Errorf("centre kept: %v", dig(resp, "data", "latitude"))
		}
		return "cleared", nil
	})
	s.r.Check("sending the new centre with the city is used as given", func() (string, error) {
		status, resp, err := s.setPrefs("ivy", map[string]any{"city": "Lahore", "latitude": 31.5, "longitude": 74.3, "job_radius_km": 25.0})
		if err != nil || status != 200 {
			return "", fmt.Errorf("status %d %v", status, err)
		}
		lat, _ := num(resp, "data", "latitude")
		return fmt.Sprint(lat), expectFloat(lat, 31.5)
	})
}

func expectFloat(got, want float64) error {
	if got < want-0.001 || got > want+0.001 {
		return fmt.Errorf("got %v, want %v", got, want)
	}
	return nil
}
