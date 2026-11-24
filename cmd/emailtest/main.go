// emailtest checks every email the Tasksy app sends: it runs the real flows that
// trigger them (proposal received/accepted/rejected, job completed, payment
// released), the admin and send endpoints, and each template, then verifies who
// got what.
//
// Sink mode (default, local only) catches every email in a built-in SMTP server
// and checks recipients, subjects, bodies and headers:
//
//	go run ./cmd/emailtest -db "$DB_URI"
//
// The server must be running against that same DB. The tool points the
// noreply@tasksy.co.uk account and the six templates at the sink for the run
// and puts the originals back afterwards.
//
// Real mode sends everything through the server's configured SMTP account to
// your test inbox (users are created as inbox+tag@domain); check the inbox for
// the list printed at the end:
//
//	go run ./cmd/emailtest -base-url https://staging.example -to qa@example.com -admin-token ...
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"tasksy/internal/modules/email"
)

type User struct {
	Label, First, Last, Email, Password, ID, Token string
}

func (u *User) Name() string { return u.First + " " + u.Last }

type Suite struct {
	api        *API
	r          *Runner
	sink       *Sink
	db         *testDB
	mailer     *email.Service
	adminToken string
	run        int64
	realTo     string
	catID      string
	sent       []string
}

func (s *Suite) real() bool { return s.realTo != "" }

func main() { os.Exit(run()) }

func run() int {
	baseURL := flag.String("base-url", "http://localhost:8080", "Tasksy API base URL")
	dsn := flag.String("db", os.Getenv("EMAILTEST_DB"), "Postgres DSN of the server's database (sink mode; local DB only)")
	realTo := flag.String("to", "", "your test inbox; sends real emails through the server's SMTP account instead of using the sink")
	adminToken := flag.String("admin-token", os.Getenv("ADMIN_ACCESS_TOKEN"), "admin access token (real mode)")
	allowProd := flag.Bool("allow-production", false, "allow targeting a tasksy.co.uk server (creates test users and jobs there)")
	flag.Parse()

	if strings.Contains(*baseURL, "tasksy.co.uk") && !*allowProd {
		fmt.Println("refusing to run against a production server: this creates test users and jobs. Use a local or staging server, or pass -allow-production.")
		return 2
	}

	s := &Suite{api: newAPI(*baseURL), r: &Runner{}, run: time.Now().Unix(), realTo: *realTo, adminToken: *adminToken}
	rand.Seed(s.run)

	if s.real() {
		if !strings.Contains(s.realTo, "@") {
			fmt.Println("-to must be an email address")
			return 2
		}
		if s.adminToken == "" {
			fmt.Println("real mode needs -admin-token (the tool creates a category and verifies the test freelancers through admin APIs)")
			return 2
		}
	} else {
		if *dsn == "" {
			fmt.Println("sink mode needs -db (the server's Postgres DSN), or use -to for real mode")
			return 2
		}
		if !isLocalDSN(*dsn) {
			fmt.Println("refusing to run: -db is not a local database, and sink mode redirects the server's email account while it runs. Use a local DB, or -to for real mode.")
			return 2
		}
		sink, err := startSink()
		if err != nil {
			fmt.Println("cannot start the SMTP sink:", err)
			return 2
		}
		defer sink.Close()
		s.sink = sink

		db, err := openTestDB(*dsn)
		if err != nil {
			fmt.Println("cannot open -db:", err)
			return 2
		}
		s.db = db
		defer db.Restore()
		s.mailer = email.NewService(db.g, nil)

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		go func() { <-sig; db.Restore(); os.Exit(130) }()
	}

	fmt.Printf("%sTasksy email test%s base=%s mode=%s run=%d\n", colorBold, colorReset, *baseURL, map[bool]string{true: "real send to " + s.realTo, false: "SMTP sink"}[s.real()], s.run)

	client := s.user("client", "Cora", "Clientson")
	f1 := s.user("freelancer", "Fiona", "Freelance")
	f2 := s.user("rejected", "Felix", "Rejected")

	s.setup(client, f1, f2)
	s.endpoints(client)
	if !s.real() {
		s.templates()
		s.injection(client, f1)
	}
	s.triggers(client, f1, f2)
	if !s.real() {
		s.liveFormat(client, f1)
	}
	s.finish()

	s.r.Summary()
	return s.r.ExitCode()
}

func (s *Suite) user(label, first, last string) *User {
	addr := fmt.Sprintf("emailtest-%d-%s@example.com", s.run, label)
	if s.real() {
		at := strings.LastIndex(s.realTo, "@")
		addr = fmt.Sprintf("%s+etest%d%s%s", s.realTo[:at], s.run, label, s.realTo[at:])
	}
	return &User{Label: label, First: first, Last: last, Email: addr, Password: "Password123"}
}

func (s *Suite) register(u *User) (string, error) {
	status, resp, err := s.api.Form("/api/auth/register", "", url.Values{
		"first_name": {u.First}, "last_name": {u.Last}, "email": {u.Email}, "password": {u.Password},
		"phone_number": {fmt.Sprintf("+4477%08d", rand.Intn(100000000))},
	})
	if err != nil {
		return "", err
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
	}
	u.Token = str(resp, "tokens", "access_token")
	u.ID = str(resp, "user", "id")
	if u.Token == "" || u.ID == "" {
		return "", fmt.Errorf("no token or id in response")
	}
	return u.Email, nil
}

func (s *Suite) adminLogin(email, password string) (string, error) {
	status, resp, err := s.api.JSON("POST", "/api/auth/admin/login", "", map[string]any{"email": email, "password": password})
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
	}
	return str(resp, "tokens", "access_token"), nil
}

func (s *Suite) setup(client, f1, f2 *User) {
	s.r.Section("Setup")
	if !s.real() {
		s.r.MustCheck("SMTP sink is listening and the server's email account points at it", func() (string, error) {
			if err := s.db.pointAtSink(s.sink.Port); err != nil {
				return "", err
			}
			return fmt.Sprintf("127.0.0.1:%d as %s", s.sink.Port, senderEmail), nil
		})
		s.r.MustCheck("all six email templates are installed", func() (string, error) {
			if err := s.db.seedTemplates(); err != nil {
				return "", err
			}
			names := make([]string, len(templateSpecs))
			for i, t := range templateSpecs {
				names[i] = t.name
			}
			return strings.Join(names, ", "), nil
		})
		admin := &User{Label: "admin", First: "Email", Last: "Admin", Email: fmt.Sprintf("emailtest-%d-admin@example.com", s.run), Password: "Password123"}
		s.r.MustCheck("an admin test user exists", func() (string, error) {
			if _, err := s.register(admin); err != nil {
				return "", err
			}
			if err := s.db.grantAdmin(admin.ID); err != nil {
				return "", err
			}
			tok, err := s.adminLogin(admin.Email, admin.Password)
			s.adminToken = tok
			return admin.Email, err
		})
	} else {
		s.r.MustCheck("the server has all six templates installed", func() (string, error) {
			status, list, err := s.api.List("/api/v1/admin/email-templates", s.adminToken)
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("status %d listing templates", status)
			}
			have := map[string]bool{}
			for _, t := range list {
				have[str(t, "name")] = true
			}
			var missing []string
			for _, t := range templateSpecs {
				if !have[t.name] {
					missing = append(missing, t.name)
				}
			}
			if len(missing) > 0 {
				return "", fmt.Errorf("missing %v: the app silently skips those emails", missing)
			}
			return "6 of 6", nil
		})
		s.r.MustCheck("an active noreply sender account is configured", func() (string, error) {
			status, list, err := s.api.List("/api/v1/admin/email-accounts", s.adminToken)
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("status %d listing accounts", status)
			}
			for _, a := range list {
				if str(a, "email") == senderEmail && dig(a, "is_active") == true {
					return fmt.Sprintf("%s via %s:%v", senderEmail, str(a, "host"), a["port"]), nil
				}
			}
			return "", fmt.Errorf("no active %s account: no email can be sent", senderEmail)
		})
	}

	s.r.MustCheck("register client, freelancer and a second freelancer", func() (string, error) {
		for _, u := range []*User{client, f1, f2} {
			if _, err := s.register(u); err != nil {
				return "", fmt.Errorf("%s: %w", u.Label, err)
			}
		}
		return fmt.Sprintf("%s, %s, %s", client.Email, f1.Email, f2.Email), nil
	})
	s.r.MustCheck("verify both freelancers and create a category", func() (string, error) {
		for _, u := range []*User{f1, f2} {
			status, resp, err := s.api.JSON("PUT", "/api/v1/admin/users/"+u.ID, s.adminToken, map[string]any{"identity_verified": true})
			if err != nil {
				return "", err
			}
			if status != 200 {
				return "", fmt.Errorf("verify %s: status %d: %s", u.Label, status, errMsg(resp))
			}
		}
		status, resp, err := s.api.JSON("POST", "/api/v1/category/create", s.adminToken, map[string]any{"name": fmt.Sprintf("EmailTest-%d", s.run)})
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("category: status %d: %s", status, errMsg(resp))
		}
		s.catID = str(resp, "data", "id")
		return s.catID, nil
	})
}
