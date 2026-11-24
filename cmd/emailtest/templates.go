package main

import (
	"fmt"
	"strings"

	"tasksy/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// templateSpec is one email the app sends. vars are exactly the placeholders the
// code fills in (controllers/proposal.go and controllers/jobs.go).
type templateSpec struct {
	name, subject, body string
	vars                map[string]string
	recipient           string // who gets it, for the report
}

var templateSpecs = []templateSpec{
	{
		name:      "proposal_received",
		subject:   "New proposal received for {{job_title}}",
		body:      "<p>Hi {{first_name}},</p><p><strong>{{freelancer_name}}</strong> sent a proposal for <strong>{{job_title}}</strong>.</p><p>Open Tasksy to review it.</p>",
		vars:      map[string]string{"first_name": "Cora", "freelancer_name": "Fiona Freelance", "job_title": "Fix my garden gate"},
		recipient: "the job poster",
	},
	{
		name:      "proposal_rejected",
		subject:   "Your proposal for {{job_title}} was not selected",
		body:      "<p>Hi {{first_name}},</p><p>The client chose another freelancer for <strong>{{job_title}}</strong>.</p>",
		vars:      map[string]string{"first_name": "Felix", "job_title": "Fix my garden gate"},
		recipient: "the freelancer whose proposal was rejected",
	},
	{
		name:      "proposal_accepted",
		subject:   "Your proposal for {{job_title}} was accepted",
		body:      "<p>Hi {{first_name}},</p><p>Good news: your proposal for <strong>{{job_title}}</strong> was accepted.</p>",
		vars:      map[string]string{"first_name": "Fiona", "job_title": "Fix my garden gate"},
		recipient: "the freelancer whose proposal was accepted",
	},
	{
		name:      "job_completed_client",
		subject:   "Your job {{job_title}} is complete",
		body:      "<p>Hi {{first_name}},</p><p><strong>{{freelancer_name}}</strong> and you have both marked <strong>{{job_title}}</strong> as complete.</p>",
		vars:      map[string]string{"first_name": "Cora", "freelancer_name": "Fiona Freelance", "job_title": "Fix my garden gate"},
		recipient: "the client, when both sides complete",
	},
	{
		name:      "job_completed_freelancer",
		subject:   "You completed {{job_title}}",
		body:      "<p>Hi {{first_name}},</p><p>You and <strong>{{client_name}}</strong> have both marked <strong>{{job_title}}</strong> as complete.</p>",
		vars:      map[string]string{"first_name": "Fiona", "client_name": "Cora Clientson", "job_title": "Fix my garden gate"},
		recipient: "the freelancer, when both sides complete",
	},
	{
		name:      "payment_released",
		subject:   "Payment released for {{job_title}}",
		body:      "<p>Hi {{first_name}},</p><p>Your payment for <strong>{{job_title}}</strong> has been released to your account.</p>",
		vars:      map[string]string{"first_name": "Fiona", "job_title": "Fix my garden gate"},
		recipient: "the freelancer, when the payment is released",
	},
}

func render(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

// testDB points the server's email setup at the sink for the run and puts the
// original account and templates back afterwards.
type testDB struct {
	g        *gorm.DB
	restore  []func()
	origAcct *models.EmailAccount
}

func isLocalDSN(dsn string) bool {
	return strings.Contains(dsn, "host=127.0.0.1") || strings.Contains(dsn, "host=localhost") ||
		strings.Contains(dsn, "@127.0.0.1") || strings.Contains(dsn, "@localhost")
}

func openTestDB(dsn string) (*testDB, error) {
	g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	return &testDB{g: g}, nil
}

func (d *testDB) Restore() {
	for i := len(d.restore) - 1; i >= 0; i-- {
		d.restore[i]()
	}
}

const senderEmail = "noreply@tasksy.co.uk"

func (d *testDB) pointAtSink(port int) error {
	var acct models.EmailAccount
	err := d.g.Unscoped().Where("email = ?", senderEmail).First(&acct).Error
	if err == nil {
		saved := acct
		d.restore = append(d.restore, func() { d.g.Unscoped().Save(&saved) })
		return d.g.Unscoped().Model(&acct).Updates(map[string]any{
			"host": "127.0.0.1", "port": port, "password": "sink", "is_active": true, "is_default": true, "deleted_at": nil,
		}).Error
	}
	acct = models.EmailAccount{Name: "emailtest-sink", Host: "127.0.0.1", Port: port, Email: senderEmail, Password: "sink", FromName: "Tasksy", IsActive: true, IsDefault: true}
	if err := d.g.Create(&acct).Error; err != nil {
		return err
	}
	d.restore = append(d.restore, func() { d.g.Unscoped().Delete(&models.EmailAccount{}, "id = ?", acct.ID) })
	return nil
}

func (d *testDB) seedTemplates() error {
	for _, t := range templateSpecs {
		var existing models.EmailTemplate
		err := d.g.Unscoped().Where("name = ?", t.name).First(&existing).Error
		if err == nil {
			saved := existing
			d.restore = append(d.restore, func() { d.g.Unscoped().Save(&saved) })
			if err := d.g.Unscoped().Model(&existing).Updates(map[string]any{"subject": t.subject, "body": t.body, "deleted_at": nil}).Error; err != nil {
				return err
			}
			continue
		}
		row := models.EmailTemplate{Name: t.name, Subject: t.subject, Body: t.body}
		if err := d.g.Create(&row).Error; err != nil {
			return fmt.Errorf("seed %s: %w", t.name, err)
		}
		id := row.ID
		d.restore = append(d.restore, func() { d.g.Unscoped().Delete(&models.EmailTemplate{}, "id = ?", id) })
	}
	return nil
}

func (d *testDB) grantAdmin(userID string) error {
	if err := d.g.Exec(`INSERT INTO roles (name) VALUES ('admin') ON CONFLICT DO NOTHING`).Error; err != nil {
		return err
	}
	if err := d.g.Exec(`INSERT INTO user_roles (user_id, role_name) VALUES (?, 'admin') ON CONFLICT DO NOTHING`, userID).Error; err != nil {
		return err
	}
	d.restore = append(d.restore, func() {
		d.g.Exec(`DELETE FROM user_roles WHERE user_id = ? AND role_name = 'admin'`, userID)
	})
	return nil
}

func (d *testDB) setTemplate(name, subject, body string) error {
	return d.g.Unscoped().Model(&models.EmailTemplate{}).Where("name = ?", name).
		Updates(map[string]any{"subject": subject, "body": body}).Error
}
