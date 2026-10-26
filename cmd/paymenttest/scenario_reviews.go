package main

import (
	"fmt"
	"strings"
	"time"
)

func (s *Suite) review(u *TestUser, contractID string, rating int) (int, map[string]any, error) {
	return s.api.Do("POST", "/api/contracts/"+contractID+"/review", u.AccessToken, map[string]any{
		"rating": rating, "comment": fmt.Sprintf("paymenttest review (%d stars)", rating),
	}, nil)
}

type userRating struct{ rating, count float64 }

func ratingOf(m map[string]any) userRating {
	return userRating{getNum(m, "rating"), getNum(m, "reviews_count")}
}

func (s *Suite) reviewFlows(client, f1, f2 *TestUser, a, d, pf *Flow) {
	s.r.Section("Reviews — ratings and photos appear on every endpoint")

	if s.db != nil {
		s.r.MustCheck("give both reviewers a profile photo", func() (string, error) {
			if err := s.db.setProfilePhoto(client.ID, "https://example.com/client.jpg"); err != nil {
				return "", err
			}
			return "photos set", s.db.setProfilePhoto(f1.ID, "https://example.com/freelancer.jpg")
		})
	}

	s.r.Check("rating outside 1-5 is refused", func() (string, error) {
		for _, bad := range []int{0, 6} {
			status, resp, err := s.review(client, a.ContractID, bad)
			if err != nil {
				return "", err
			}
			if _, err := expectRefused(status, resp, "rating"); err != nil {
				return "", fmt.Errorf("rating %d: %w", bad, err)
			}
		}
		return "0 and 6 refused", nil
	})
	s.r.Check("outsider cannot review the contract", func() (string, error) {
		status, resp, err := s.review(f2, a.ContractID, 5)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", status), expect(status, resp, 403)
	})
	s.r.Check("cannot review a contract that isn't completed", func() (string, error) {
		open := s.setupFlowQuiet(client, f1)
		status, resp, err := s.review(client, open.ContractID, 5)
		if err != nil {
			return "", err
		}
		return expectRefused(status, resp, "completed")
	})

	s.r.MustCheck("client rates the freelancer 5 and the freelancer rates the client 4", func() (string, error) {
		for _, c := range []struct {
			u      *TestUser
			rating int
		}{{client, 5}, {f1, 4}} {
			status, resp, err := s.review(c.u, a.ContractID, c.rating)
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 200, 201); err != nil {
				return "", fmt.Errorf("%s: %w", c.u.Label, err)
			}
		}
		return "both reviews saved", nil
	})
	s.r.Check("reviewing the same contract twice is refused (409)", func() (string, error) {
		status, resp, err := s.review(client, a.ContractID, 1)
		if err != nil {
			return "", err
		}
		return errMsg(resp), expect(status, resp, 409)
	})

	want := func(who string, got userRating, rating, count float64) error {
		if !eq(got.rating, rating) || !eq(got.count, count) {
			return fmt.Errorf("%s shows rating=%.2f reviews_count=%.0f, want %.2f / %.0f", who, got.rating, got.count, rating, count)
		}
		return nil
	}

	s.r.Check("GET /api/v1/job/:id shows the job poster's rating and review count", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v1/job/"+a.JobID, f1.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		return "poster 4.0 (1)", want("created_by", ratingOf(getMap(resp, "data", "created_by")), 4, 1)
	})
	s.r.Check("GET /api/job/:id/proposal shows each freelancer's rating and review count", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/job/"+a.JobID+"/proposal", client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		rows := getRows(resp, "data")
		if len(rows) == 0 {
			return "", fmt.Errorf("no proposals returned")
		}
		return "freelancer 5.0 (1)", want("freelancer", ratingOf(getMap(rows[0], "freelancer")), 5, 1)
	})
	s.r.Check("GET /api/proposals/:id shows both users' rating and review count", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/proposals/"+a.ProposalID, client.AccessToken, nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		if err := want("freelancer", ratingOf(getMap(resp, "data", "freelancer")), 5, 1); err != nil {
			return "", err
		}
		return "freelancer 5.0 (1), poster 4.0 (1)", want("job poster", ratingOf(getMap(resp, "data", "job_post", "created_by")), 4, 1)
	})
	s.r.Check("job feed shows the poster's rating and review count", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v1/job/feed?limit=100", "", nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		rows := getRows(resp, "data")
		if len(rows) == 0 {
			rows = getRows(resp, "data", "jobs")
		}
		for _, r := range rows {
			if getStr(r, "created_by", "id") == client.ID {
				return "poster 4.0 (1)", want("feed poster", ratingOf(getMap(r, "created_by")), 4, 1)
			}
		}
		return "no job by the test client in the feed", nil
	})

	s.r.Check("review list shows who wrote each review, with their photo", func() (string, error) {
		status, resp, err := s.api.Do("GET", "/api/v1/user/"+f1.ID+"/profile", "", nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		root := resp
		if d := getMap(resp, "data"); d != nil {
			root = d
		}
		if err := want("profile", ratingOf(getMap(root, "user")), 5, 1); err != nil {
			return "", err
		}
		reviews := getRows(root, "reviews")
		if len(reviews) != 1 {
			return "", fmt.Errorf("%d reviews listed", len(reviews))
		}
		by := getMap(reviews[0], "reviewed_by")
		if getStr(by, "id") != client.ID || getStr(by, "first_name") == "" {
			return "", fmt.Errorf("reviewed_by=%v", by)
		}
		if _, ok := by["profile_photo"]; !ok {
			return "", fmt.Errorf("reviewed_by has no profile_photo field")
		}
		if s.db != nil && getStr(by, "profile_photo") != "https://example.com/client.jpg" {
			return "", fmt.Errorf("profile_photo=%q", getStr(by, "profile_photo"))
		}
		return "reviewed_by " + getStr(by, "first_name") + " with photo " + getStr(by, "profile_photo"), nil
	})

	s.r.Check("a second review averages with the first (5 and 2 -> 3.5 over 2)", func() (string, error) {
		for _, c := range []struct {
			f      *Flow
			rating int
		}{{d, 5}, {pf, 2}} {
			status, resp, err := s.review(client, c.f.ContractID, c.rating)
			if err != nil {
				return "", err
			}
			if err := expect(status, resp, 200, 201); err != nil {
				return "", err
			}
		}
		status, resp, err := s.api.Do("GET", "/api/v1/user/"+f2.ID+"/profile", "", nil, nil)
		if err != nil {
			return "", err
		}
		if err := expect(status, resp, 200); err != nil {
			return "", err
		}
		root := resp
		if dd := getMap(resp, "data"); dd != nil {
			root = dd
		}
		reviews := getRows(root, "reviews")
		var photos []string
		for _, r := range reviews {
			photos = append(photos, getStr(r, "reviewed_by", "profile_photo"))
		}
		if err := want("profile", ratingOf(getMap(root, "user")), 3.5, 2); err != nil {
			return "", err
		}
		return fmt.Sprintf("3.5 over 2, reviewer photos %s", strings.Join(photos, ",")), nil
	})
}

func (s *Suite) setupFlowQuiet(client, freelancer *TestUser) *Flow {
	f := &Flow{Client: client, Freelancer: freelancer}
	id, err := s.createJob(client, "paytest unfinished", 5)
	if err != nil {
		return f
	}
	status, resp, _ := s.sendProposal(freelancer, id, 5, nil)
	if status != 200 && status != 201 {
		return f
	}
	f.ProposalID = getStr(resp, "data", "id")
	_, _, _ = s.decide(client.AccessToken, f.ProposalID, "accepted")
	_, cresp, _ := s.createContract(client.AccessToken, f.ProposalID, 5, time.Now(), time.Now().Add(24*time.Hour))
	f.ContractID = getStr(cresp, "contract", "id")
	return f
}
