// notifytest checks radius-based new-job notifications through the real API:
// users choose a city and radius in /api/v1/notifications/preferences, jobs are
// posted at different places, and each user's notification list must contain
// exactly the jobs inside their radius.
//
//	go run ./cmd/notifytest [-base-url http://localhost:8080]
//
// Needs a local or staging server; it creates users and jobs.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"
)

const prefsPath = "/api/v1/notifications/preferences"

type user struct {
	name, email, id, token string
}

type job struct {
	key, title, city string
	lat, lng         float64
	id               string
}

type suite struct {
	api   *API
	r     *Runner
	run   int64
	users map[string]*user
	jobs  map[string]*job
	cat   string
}

func main() {
	base := flag.String("base-url", "http://localhost:8080", "Tasksy API base URL")
	allowProd := flag.Bool("allow-production", false, "allow a tasksy.co.uk server")
	flag.Parse()
	if strings.Contains(*base, "tasksy.co.uk") && !*allowProd {
		fmt.Println("refusing to run against production: this creates users and jobs")
		os.Exit(2)
	}
	s := &suite{api: newAPI(*base), r: &Runner{}, run: time.Now().Unix(), users: map[string]*user{}, jobs: map[string]*job{}}
	rand.Seed(s.run)
	fmt.Printf("%sRadius notification test%s base=%s run=%d\n", colorBold, colorReset, *base, s.run)

	s.setup()
	s.validation()
	s.matching()
	s.cityChange()
	s.r.Summary()
	os.Exit(s.r.ExitCode())
}

func (s *suite) register(name string) (string, error) {
	u := &user{name: name, email: fmt.Sprintf("notify-%d-%s@example.com", s.run, name)}
	status, resp, err := s.api.Form("/api/auth/register", "", url.Values{
		"first_name": {strings.Title(name)}, "last_name": {"Tester"}, "email": {u.email}, "password": {"Password123"},
		"phone_number": {fmt.Sprintf("+4477%08d", rand.Intn(100000000))},
	})
	if err != nil {
		return "", err
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
	}
	u.token, u.id = str(resp, "tokens", "access_token"), str(resp, "user", "id")
	s.users[name] = u
	return u.id, nil
}

func (s *suite) setPrefs(name string, body map[string]any) (int, map[string]any, error) {
	return s.api.JSON("PUT", prefsPath, s.users[name].token, body)
}

func (s *suite) postJob(poster, key, city string, lat, lng float64) error {
	j := &job{key: key, title: fmt.Sprintf("Radius %s %d", key, s.run), city: city, lat: lat, lng: lng}
	form := url.Values{
		"category_id": {s.cat}, "title": {j.title}, "description": {"radius notification test"},
		"budget": {"20.00"}, "open_budget": {"false"}, "city": {city}, "state": {"Test State"}, "country": {"Test Country"},
	}
	if lat != 0 || lng != 0 {
		form.Set("latitude", fmt.Sprint(lat))
		form.Set("longitude", fmt.Sprint(lng))
	}
	status, resp, err := s.api.Form("/api/job/create", s.users[poster].token, form)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("job %s: status %d: %s", key, status, errMsg(resp))
	}
	j.id = str(resp, "data", "id")
	s.jobs[key] = j
	return nil
}

// notified returns the jobs (by key) a user has been notified about, with the
// notification body for each.
func (s *suite) notified(name string) (map[string]string, error) {
	status, resp, err := s.api.JSON("GET", "/api/v1/notifications?limit=100", s.users[name].token, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d", status)
	}
	out := map[string]string{}
	list, _ := resp["data"].([]any)
	for _, it := range list {
		n, _ := it.(map[string]any)
		if str(n, "type") != "new_job" {
			continue
		}
		id := str(n, "data", "job_post_id")
		for key, j := range s.jobs {
			if j.id == id {
				out[key] = str(n, "body")
			}
		}
	}
	return out, nil
}
