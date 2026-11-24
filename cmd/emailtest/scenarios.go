package main

import (
	"fmt"
	"mime"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const wait = 20 * time.Second

func spec(name string) templateSpec {
	for _, t := range templateSpecs {
		if t.name == name {
			return t
		}
	}
	panic("unknown template " + name)
}

// verify checks what every email must satisfy, whichever flow sent it.
func (s *Suite) verify(m *Mail, wantRcpt string) error {
	var problems []string
	if len(m.Rcpts) != 1 || !strings.EqualFold(m.Rcpts[0], wantRcpt) {
		problems = append(problems, fmt.Sprintf("recipients %v, want only %s", m.Rcpts, wantRcpt))
	}
	if m.EnvelopeFrom != senderEmail {
		problems = append(problems, "envelope sender "+m.EnvelopeFrom)
	}
	if from := m.Header.Get("From"); !strings.Contains(from, "<"+senderEmail+">") {
		problems = append(problems, "From header "+from)
	}
	if ct := m.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") || !strings.Contains(strings.ToLower(ct), "utf-8") {
		problems = append(problems, "Content-Type "+ct)
	}
	if m.Header.Get("Mime-Version") == "" {
		problems = append(problems, "no MIME-Version")
	}
	if strings.Contains(m.Subject(), "{{") || strings.Contains(m.Body, "{{") {
		problems = append(problems, "a {{placeholder}} was left unreplaced")
	}
	if strings.TrimSpace(m.Subject()) == "" || strings.TrimSpace(m.Body) == "" {
		problems = append(problems, "empty subject or body")
	}
	if m.Header.Get("Bcc") != "" || m.Header.Get("Cc") != "" {
		problems = append(problems, "unexpected Cc/Bcc header")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// expect waits for one email of a template to reach rcpt. frags must appear in
// the body; the first one also picks the email when several share a subject.
func (s *Suite) expect(t templateSpec, rcpt *User, vars map[string]string, frags ...string) (string, error) {
	subject := render(t.subject, vars)
	if s.real() {
		s.sent = append(s.sent, fmt.Sprintf("%-26s to %-42s (%s)", t.name, rcpt.Email, t.recipient))
		return "sent; check the inbox", nil
	}
	m := s.sink.Wait(wait, func(m *Mail) bool {
		return to(rcpt.Email)(m) && m.Subject() == subject && (len(frags) == 0 || strings.Contains(m.Body, frags[0]))
	})
	if m == nil {
		return "", fmt.Errorf("no %q email with subject %q reached %s within %s (the server logs show why; the template must exist and the noreply account be active)", t.name, subject, rcpt.Email, wait)
	}
	if err := s.verify(m, rcpt.Email); err != nil {
		return "", err
	}
	for _, f := range frags {
		if !strings.Contains(m.Body, f) {
			return "", fmt.Errorf("body is missing %q: %s", f, m.Body)
		}
	}
	return fmt.Sprintf("%q to %s", m.Subject(), rcpt.Email), nil
}

func (s *Suite) expectNone(name string, d time.Duration, match func(*Mail) bool) (string, error) {
	if s.real() {
		return "not checked in real mode", nil
	}
	if m := s.sink.Wait(d, match); m != nil {
		return "", fmt.Errorf("an unexpected email was sent: %q to %v", m.Subject(), m.Rcpts)
	}
	return "none sent", nil
}

// ---- endpoints --------------------------------------------------------------

func (s *Suite) endpoints(client *User) {
	s.r.Section("Send endpoints")
	target := "endpoint-test@example.com"
	if s.real() {
		target = s.realTo
	}

	s.r.Check("admin test-email endpoint sends the SMTP test message", func() (string, error) {
		status, resp, err := s.api.JSON("POST", "/api/v1/admin/email/test", s.adminToken, map[string]any{"to": target})
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		if s.real() {
			s.sent = append(s.sent, fmt.Sprintf("%-26s to %-42s (SMTP test message)", "Tasksy Test Email", target))
			return "sent; check the inbox", nil
		}
		m := s.sink.Wait(wait, toWithSubject(target, "Tasksy Test Email"))
		if m == nil {
			return "", fmt.Errorf("200 returned but no email arrived")
		}
		return m.Subject(), s.verify(m, target)
	})
	s.r.Check("test-email endpoint refuses no login (401) and a normal user (403)", func() (string, error) {
		for _, c := range []struct {
			token string
			want  int
		}{{"", 401}, {client.Token, 403}} {
			status, resp, err := s.api.JSON("POST", "/api/v1/admin/email/test", c.token, map[string]any{"to": target})
			if err != nil {
				return "", err
			}
			if status != c.want {
				return "", fmt.Errorf("got %d, want %d: %s", status, c.want, errMsg(resp))
			}
		}
		return "401 and 403", nil
	})
	s.r.Check("test-email endpoint without a recipient is refused (400)", func() (string, error) {
		status, resp, err := s.api.JSON("POST", "/api/v1/admin/email/test", s.adminToken, map[string]any{})
		if err != nil {
			return "", err
		}
		return errMsg(resp), expectStatus(status, 400)
	})

	s.r.Check("send endpoint delivers a custom email as an admin", func() (string, error) {
		subject := fmt.Sprintf("Custom message %d", s.run)
		status, resp, err := s.api.JSON("POST", "/api/v1/email/send", s.adminToken, map[string]any{
			"receiver": target, "subject": subject, "content": "<p>Hello from the send endpoint</p>",
		})
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		if s.real() {
			s.sent = append(s.sent, fmt.Sprintf("%-26s to %-42s (custom send)", subject, target))
			return "sent; check the inbox", nil
		}
		m := s.sink.Wait(wait, toWithSubject(target, subject))
		if m == nil {
			return "", fmt.Errorf("200 returned but no email arrived")
		}
		if !strings.Contains(m.Body, "Hello from the send endpoint") {
			return "", fmt.Errorf("body not delivered: %q", m.Body)
		}
		return subject, s.verify(m, target)
	})
	s.r.Check("send endpoint refuses bad input (400)", func() (string, error) {
		for _, body := range []map[string]any{
			{"receiver": "not-an-email", "subject": "x", "content": "y"},
			{"receiver": target, "content": "y"},
			{"receiver": target, "subject": "x"},
			{},
		} {
			status, resp, err := s.api.JSON("POST", "/api/v1/email/send", s.adminToken, body)
			if err != nil {
				return "", err
			}
			if status != 400 {
				return "", fmt.Errorf("%v: got %d: %s", body, status, errMsg(resp))
			}
		}
		return "4 bad bodies refused", nil
	})
	if !s.real() {
		s.r.Warn("an anonymous caller can't send email as noreply@tasksy.co.uk", func() (string, error) {
			status, _, err := s.api.JSON("POST", "/api/v1/email/send", "", map[string]any{
				"receiver": "anonymous-relay@example.com", "subject": "anonymous", "content": "<p>sent without logging in</p>",
			})
			if err != nil {
				return "", err
			}
			if status == 401 || status == 403 {
				return fmt.Sprintf("refused (%d)", status), nil
			}
			s.sink.Wait(5*time.Second, to("anonymous-relay@example.com"))
			return "", fmt.Errorf("answered %d and sent it: anyone on the internet can send mail from the company address to any recipient (phishing). If the app needs this open, restrict who can call it", status)
		})
	}
}

func expectStatus(got, want int) error {
	if got != want {
		return fmt.Errorf("status %d, want %d", got, want)
	}
	return nil
}

// ---- every template, delivered directly ----------------------------------------

func (s *Suite) templates() {
	s.r.Section("All six templates render and deliver")
	for _, t := range templateSpecs {
		t := t
		s.r.Check(t.name+": delivered with every placeholder filled in", func() (string, error) {
			rcpt := fmt.Sprintf("direct-%s-%d@example.com", t.name, s.run)
			if err := s.mailer.SendTemplatedEmail(t.name, rcpt, t.vars); err != nil {
				return "", err
			}
			m := s.sink.Wait(wait, to(rcpt))
			if m == nil {
				return "", fmt.Errorf("no email arrived")
			}
			if err := s.verify(m, rcpt); err != nil {
				return "", err
			}
			if want := render(t.subject, t.vars); m.Subject() != want {
				return "", fmt.Errorf("subject %q, want %q", m.Subject(), want)
			}
			if want := render(t.body, t.vars); strings.TrimSpace(m.Body) != want {
				return "", fmt.Errorf("body %q, want %q", strings.TrimSpace(m.Body), want)
			}
			return m.Subject(), nil
		})
	}
	s.r.Check("an unknown template is an error and sends nothing", func() (string, error) {
		before := s.sink.Total()
		err := s.mailer.SendTemplatedEmail("no_such_template", "ghost@example.com", map[string]string{})
		if err == nil {
			return "", fmt.Errorf("no error for a missing template")
		}
		time.Sleep(500 * time.Millisecond)
		if s.sink.Total() != before {
			return "", fmt.Errorf("an email was sent for a missing template")
		}
		return err.Error(), nil
	})
}

// ---- hostile input ---------------------------------------------------------------

var rawSubjectLine = regexp.MustCompile(`(?im)^subject:[^\r\n]*`)

func (s *Suite) injection(client, f1 *User) {
	s.r.Section("Hostile input in names and titles can't break the emails")
	tmpl := spec("proposal_received")

	send := func(title string) (*Mail, string, error) {
		rcpt := fmt.Sprintf("hostile-%d-%d@example.com", s.run, s.sink.Total())
		vars := map[string]string{"first_name": "Cora", "freelancer_name": "Fiona Freelance", "job_title": title}
		if err := s.mailer.SendTemplatedEmail(tmpl.name, rcpt, vars); err != nil {
			return nil, "", err
		}
		m := s.sink.Wait(wait, to(rcpt))
		if m == nil {
			return nil, "", fmt.Errorf("no email arrived")
		}
		return m, rcpt, nil
	}

	s.r.Check("HTML in a job title is shown as text, never as markup", func() (string, error) {
		m, _, err := send(`<script>alert(1)</script> & <a href="https://evil.test">click</a>`)
		if err != nil {
			return "", err
		}
		for _, bad := range []string{"<script>", `<a href="https://evil.test">`} {
			if strings.Contains(m.Body, bad) {
				return "", fmt.Errorf("the job title's HTML reached the email body unescaped (%q): anyone can put links or forms into emails sent to other users", bad)
			}
		}
		if !strings.Contains(m.Body, "&lt;script&gt;") {
			return "", fmt.Errorf("title not shown as text: %s", m.Body)
		}
		return "escaped", nil
	})
	s.r.Check("a line break in a job title can't add email headers or a second message", func() (string, error) {
		m, rcpt, err := send("Garden gate\r\nBcc: attacker@evil.test\r\nX-Evil: 1\r\n\r\nPay me at evil.test")
		if err != nil {
			return "", err
		}
		if m.Header.Get("Bcc") != "" || m.Header.Get("X-Evil") != "" {
			return "", fmt.Errorf("headers were injected through the subject: Bcc=%q X-Evil=%q", m.Header.Get("Bcc"), m.Header.Get("X-Evil"))
		}
		if strings.Contains(m.Body, "Pay me at evil.test") && !strings.Contains(m.Body, "Garden gate") {
			return "", fmt.Errorf("the title's text took over the email body")
		}
		if len(m.Rcpts) != 1 || !strings.EqualFold(m.Rcpts[0], rcpt) {
			return "", fmt.Errorf("recipients %v", m.Rcpts)
		}
		return "one clean message", nil
	})
	s.r.Check("a subject with £, é or ✓ is encoded so every mail client shows it correctly", func() (string, error) {
		title := "Café £12 ✓"
		m, _, err := send(title)
		if err != nil {
			return "", err
		}
		line := rawSubjectLine.FindString(m.Raw)
		for _, r := range line {
			if r > 127 {
				return "", fmt.Errorf("the Subject header carries raw non-ASCII characters (%q): many clients show them garbled", line)
			}
		}
		decoded, err := new(mime.WordDecoder).DecodeHeader(m.Subject())
		if err != nil || decoded != render(tmpl.subject, map[string]string{"job_title": title}) {
			return "", fmt.Errorf("decoded subject %q", decoded)
		}
		return decoded, nil
	})
	s.r.Check("the same hostile title through the real app flow is also safe", func() (string, error) {
		title := "Hostile <b>bold</b> & co\r\nBcc: attacker@evil.test"
		status, resp, err := s.api.Form("/api/job/create", client.Token, url.Values{
			"category_id": {s.catID}, "title": {title}, "description": {"hostile title test"},
			"budget": {"10.00"}, "open_budget": {"false"},
		})
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("job create: status %d: %s", status, errMsg(resp))
		}
		jobID := str(resp, "data", "id")
		if st, r2, err := s.api.JSON("POST", "/api/job/send-proposal", f1.Token, map[string]any{
			"job_post_id": jobID, "cover_letter": "hostile", "bid_amount": 5, "duration": 1,
		}); err != nil || (st != 200 && st != 201) {
			return "", fmt.Errorf("proposal: %d %v %s", st, err, errMsg(r2))
		}
		m := s.sink.Wait(wait, func(m *Mail) bool {
			return to(client.Email)(m) && strings.HasPrefix(m.Subject(), "New proposal received for Hostile")
		})
		if m == nil {
			return "", fmt.Errorf("no email arrived")
		}
		if m.Header.Get("Bcc") != "" || strings.Contains(m.Body, "<b>bold</b>") {
			return "", fmt.Errorf("Bcc=%q, raw HTML in body=%v", m.Header.Get("Bcc"), strings.Contains(m.Body, "<b>bold</b>"))
		}
		return "clean", s.verify(m, client.Email)
	})
	s.r.Warn("every email carries Date and Message-ID headers", func() (string, error) {
		m, _, err := send("Header check")
		if err != nil {
			return "", err
		}
		if m.Header.Get("Date") == "" || m.Header.Get("Message-Id") == "" {
			return "", fmt.Errorf("missing Date=%v Message-ID=%v; Gmail and Microsoft add them, but a missing Message-ID or Date raises the spam score with strict receivers", m.Header.Get("Date") == "", m.Header.Get("Message-Id") == "")
		}
		return "present", nil
	})
}

// ---- the real flows ----------------------------------------------------------------

func (s *Suite) triggers(client, f1, f2 *User) {
	s.r.Section("Emails sent by the real app flow")
	title := fmt.Sprintf("Garden gate %d", s.run)

	var jobID, p1, p2, contractID string
	s.r.MustCheck("client posts a job", func() (string, error) {
		status, resp, err := s.api.Form("/api/job/create", client.Token, url.Values{
			"category_id": {s.catID}, "title": {title}, "description": {"email test job"},
			"budget": {"20.00"}, "open_budget": {"false"},
		})
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		jobID = str(resp, "data", "id")
		return jobID, nil
	})

	propose := func(u *User, bid float64) (string, error) {
		status, resp, err := s.api.JSON("POST", "/api/job/send-proposal", u.Token, map[string]any{
			"job_post_id": jobID, "cover_letter": "I can do this", "bid_amount": bid, "duration": 1,
		})
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		return str(resp, "data", "id"), nil
	}

	s.r.MustCheck("first freelancer sends a proposal", func() (string, error) {
		var err error
		p1, err = propose(f1, 10)
		return p1, err
	})
	s.r.Check("proposal_received: the client is emailed about the first freelancer", func() (string, error) {
		return s.expect(spec("proposal_received"), client, map[string]string{"first_name": client.First, "freelancer_name": f1.Name(), "job_title": title}, f1.Name(), client.First, title)
	})
	s.r.MustCheck("second freelancer sends a proposal", func() (string, error) {
		var err error
		p2, err = propose(f2, 12)
		return p2, err
	})
	s.r.Check("proposal_received: the client gets a separate email for the second freelancer", func() (string, error) {
		return s.expect(spec("proposal_received"), client, map[string]string{"first_name": client.First, "freelancer_name": f2.Name(), "job_title": title}, f2.Name(), client.First, title)
	})
	s.r.Check("the freelancers were not emailed about their own proposals", func() (string, error) {
		return s.expectNone("own proposal", 2*time.Second, func(m *Mail) bool {
			return strings.Contains(m.Subject(), "New proposal") && (to(f1.Email)(m) || to(f2.Email)(m))
		})
	})

	decide := func(proposalID, decision string) error {
		status, _, err := s.api.JSON("POST", "/api/proposals/"+proposalID+"/decision", client.Token, map[string]any{"status": decision})
		if err != nil {
			return err
		}
		return expectStatus(status, 200)
	}
	s.r.MustCheck("client rejects the second freelancer's proposal", func() (string, error) {
		return "rejected", decide(p2, "rejected")
	})
	s.r.Check("proposal_rejected: only the rejected freelancer is emailed", func() (string, error) {
		return s.expect(spec("proposal_rejected"), f2, map[string]string{"first_name": f2.First, "job_title": title}, f2.First, title)
	})
	s.r.MustCheck("client accepts the first freelancer's proposal", func() (string, error) {
		return "accepted", decide(p1, "accepted")
	})
	s.r.Check("proposal_accepted: only the accepted freelancer is emailed", func() (string, error) {
		return s.expect(spec("proposal_accepted"), f1, map[string]string{"first_name": f1.First, "job_title": title}, f1.First, title)
	})

	s.r.MustCheck("client creates the contract", func() (string, error) {
		status, resp, err := s.api.JSON("POST", "/api/proposals/create-contract", client.Token, map[string]any{
			"proposal_id": p1, "title": title, "description": "email test", "total_amount": 10,
			"start_date": time.Now().Format(time.RFC3339), "end_date": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})
		if err != nil {
			return "", err
		}
		if status != 200 && status != 201 {
			return "", fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		contractID = str(resp, "contract", "id")
		return contractID, nil
	})

	complete := func(u *User) error {
		status, resp, err := s.api.JSON("POST", "/api/contracts/"+contractID+"/complete", u.Token, map[string]any{})
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("status %d: %s", status, errMsg(resp))
		}
		return nil
	}
	s.r.MustCheck("the freelancer marks the job complete", func() (string, error) { return "one side done", complete(f1) })
	s.r.Check("no completion email is sent until both sides have completed", func() (string, error) {
		return s.expectNone("early completion", 3*time.Second, func(m *Mail) bool {
			return strings.Contains(m.Subject(), title) && (to(client.Email)(m) || to(f1.Email)(m)) &&
				(strings.Contains(m.Subject(), "complete") || strings.Contains(m.Subject(), "completed"))
		})
	})
	s.r.MustCheck("the client marks the job complete", func() (string, error) { return "both sides done", complete(client) })
	s.r.Check("job_completed_client: the client is emailed with the freelancer's name", func() (string, error) {
		return s.expect(spec("job_completed_client"), client, map[string]string{"first_name": client.First, "freelancer_name": f1.Name(), "job_title": title}, f1.Name(), client.First, title)
	})
	s.r.Check("job_completed_freelancer: the freelancer is emailed with the client's name", func() (string, error) {
		return s.expect(spec("job_completed_freelancer"), f1, map[string]string{"first_name": f1.First, "client_name": client.Name(), "job_title": title}, client.Name(), f1.First, title)
	})
	s.r.Check("payment_released: not sent when nothing was paid (no money was released)", func() (string, error) {
		return s.expectNone("payment_released", 3*time.Second, func(m *Mail) bool {
			return strings.HasPrefix(m.Subject(), "Payment released for "+title)
		})
	})
	if s.real() {
		s.r.Check("payment_released: needs a paid job, so the tool can't trigger it in real mode", func() (string, error) {
			return "covered by the template checks in sink mode and by the payment suite", nil
		})
	}
}

// ---- end of run -----------------------------------------------------------------------

func (s *Suite) finish() {
	s.r.Section("Nothing unexpected was sent")
	if s.real() {
		fmt.Printf("\n%sCheck %s for these emails:%s\n", colorBold, s.realTo, colorReset)
		for _, line := range s.sent {
			fmt.Println("  - " + line)
		}
		fmt.Println("  (plus-addressed users arrive in the same inbox; search for \"etest" + fmt.Sprint(s.run) + "\")")
		return
	}
	s.r.Check("every email the run produced was expected, and only test addresses were used", func() (string, error) {
		time.Sleep(time.Second)
		var stray []string
		for _, m := range s.sink.Unclaimed() {
			stray = append(stray, fmt.Sprintf("%q to %v", m.Subject(), m.Rcpts))
		}
		if len(stray) > 0 {
			return "", fmt.Errorf("%d unexpected email(s): %s", len(stray), strings.Join(stray, "; "))
		}
		return fmt.Sprintf("%d emails, all accounted for", s.sink.Total()), nil
	})
}

// The proposal_received template as it exists on the server: Go-style
// placeholders, and an amount the app has to supply.
const (
	liveSubject = `New Proposal for "{{.JobTitle}}"`
	liveBody    = `<p>Hi <b>{{.ClientName}}</b>,</p><p><b>{{.FreelancerName}}</b> has submitted a proposal on your job.</p>` +
		`<p>JOB<br><b>{{.JobTitle}}</b></p><p>PROPOSED AMOUNT<br><b>{{.ProposalAmount}}</b></p>`
)

func (s *Suite) liveFormat(client, f1 *User) {
	s.r.Section("Templates written with {{.Name}} placeholders (as on the server)")
	t := spec("proposal_received")
	defer func() { _ = s.db.setTemplate(t.name, t.subject, t.body) }()

	s.r.Check("the same placeholder written three ways is filled in everywhere", func() (string, error) {
		if err := s.db.setTemplate(t.name, "{{job_title}} {{.JobTitle}} {{ JobTitle }}", "<p>{{job_title}}|{{.JobTitle}}|{{ .JobTitle }}|{{JobTitle}}</p>"); err != nil {
			return "", err
		}
		rcpt := fmt.Sprintf("alias-%d@example.com", s.run)
		if err := s.mailer.SendTemplatedEmail(t.name, rcpt, map[string]string{"job_title": "Gate"}); err != nil {
			return "", err
		}
		m := s.sink.Wait(wait, to(rcpt))
		if m == nil {
			return "", fmt.Errorf("no email arrived")
		}
		if m.Subject() != "Gate Gate Gate" || strings.TrimSpace(m.Body) != "<p>Gate|Gate|Gate|Gate</p>" {
			return "", fmt.Errorf("subject %q body %q", m.Subject(), m.Body)
		}
		return m.Subject(), nil
	})
	s.r.Check("a placeholder the app doesn't fill in is dropped, never shown to the customer", func() (string, error) {
		if err := s.db.setTemplate(t.name, "Hello {{.NoSuchThing}}", "<p>Hi {{.Nope}}, {{ssn}}</p>"); err != nil {
			return "", err
		}
		rcpt := fmt.Sprintf("unknown-%d@example.com", s.run)
		if err := s.mailer.SendTemplatedEmail(t.name, rcpt, map[string]string{"job_title": "Gate"}); err != nil {
			return "", err
		}
		m := s.sink.Wait(wait, to(rcpt))
		if m == nil {
			return "", fmt.Errorf("no email arrived")
		}
		if strings.Contains(m.Subject()+m.Body, "{{") {
			return "", fmt.Errorf("raw template syntax reached the customer: %q %q", m.Subject(), m.Body)
		}
		return "blank", nil
	})
	s.r.Check("a real proposal fills the server's template: names, job and amount", func() (string, error) {
		if err := s.db.setTemplate(t.name, liveSubject, liveBody); err != nil {
			return "", err
		}
		title := fmt.Sprintf("Live format %d", s.run)
		status, resp, err := s.api.Form("/api/job/create", client.Token, url.Values{
			"category_id": {s.catID}, "title": {title}, "description": {"live template test"},
			"budget": {"50.00"}, "open_budget": {"false"},
		})
		if err != nil || (status != 200 && status != 201) {
			return "", fmt.Errorf("job create: %d %v %s", status, err, errMsg(resp))
		}
		if st, r2, err := s.api.JSON("POST", "/api/job/send-proposal", f1.Token, map[string]any{
			"job_post_id": str(resp, "data", "id"), "cover_letter": "live", "bid_amount": 37.5, "duration": 1,
		}); err != nil || (st != 200 && st != 201) {
			return "", fmt.Errorf("proposal: %d %v %s", st, err, errMsg(r2))
		}
		m := s.sink.Wait(wait, toWithSubject(client.Email, "New Proposal for"))
		if m == nil {
			return "", fmt.Errorf("no email arrived")
		}
		if err := s.verify(m, client.Email); err != nil {
			return "", err
		}
		if m.Subject() != `New Proposal for "`+title+`"` {
			return "", fmt.Errorf("subject %q", m.Subject())
		}
		for _, want := range []string{"Hi <b>" + client.First + "</b>", f1.Name(), title, "£37.50"} {
			if !strings.Contains(m.Body, want) {
				return "", fmt.Errorf("body is missing %q: %s", want, m.Body)
			}
		}
		return m.Subject() + " / " + "£37.50", nil
	})
}
