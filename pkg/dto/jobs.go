package dto

type Category struct {
	ID      string
	Name    string
	Disable bool
}

type JobPost struct {
	ID          string   `json:"id"`
	Category    Category `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Budget      float64  `json:"budget"`
	OpenBudget  bool     `json:"open_budget"`
	Address     string   `json:"address"`
	Status      string   `json:"status"`
}

type JobMedia struct {
	Thumbnail string
	URL       string
	MediaType string
	FileName  string
	FileSize  int64
}

type Proposal struct {
	JobPostID    string  `json:"job_post_id"`
	FreelancerID string  `json:"freelancer_id"`
	Freelancer   User    `json:"freelancer"`
	CoverLetter  string  `json:"cover_letter"`
	BidAmount    float64 `json:"bid_amount"`
	Duration     int     `json:"duration"`
	Status       string  `json:"status"`
}
