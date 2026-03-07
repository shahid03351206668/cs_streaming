package job

import (
	"fmt"

	"tasksy/models"
)

// JobFeedParams holds all query parameters for the job feed
type JobFeedParams struct {
	Category    string
	SearchQuery string
	Page        int
	Limit       int
	Latitude    *float64 // nil means no location filter
	Longitude   *float64
	RadiusKM    float64
}

// Haversine SQL expression to calculate distance in km between two lat/lng points.
// Uses the earth's mean radius of 6371 km.
// haversineWhere uses ? placeholders for GORM's Where() parameter binding.
const haversineWhere = `
	( 6371 * acos(
		cos(radians(?)) * cos(radians(jpl.latitude)) *
		cos(radians(jpl.longitude) - radians(?)) +
		sin(radians(?)) * sin(radians(jpl.latitude))
	) )
`

// haversineSelect uses %f for fmt.Sprintf since GORM Select() inlines the expression.
const haversineSelect = `
	( 6371 * acos(
		cos(radians(%f)) * cos(radians(jpl.latitude)) *
		cos(radians(jpl.longitude) - radians(%f)) +
		sin(radians(%f)) * sin(radians(jpl.latitude))
	) )
`

func (s *Service) GetJobFeed(params JobFeedParams) ([]JobPostValue, int64, error) {
	var total int64

	useLocation := params.Latitude != nil && params.Longitude != nil
	jobQuery := s.db.Model(&models.JobPost{}).Where("job_posts.status = ?", models.JobStatusOpen)

	if useLocation {
		jobQuery = jobQuery.
			Joins("JOIN job_post_locations jpl ON jpl.job_post_id = job_posts.id").
			Where(haversineWhere+" <= ?", *params.Latitude, *params.Longitude, *params.Latitude, params.RadiusKM)
	}

	if params.Category != "" {
		jobQuery = jobQuery.Where("job_posts.category_id = ?", params.Category)
	}

	if params.SearchQuery != "" {
		like := "%" + params.SearchQuery + "%"
		jobQuery = jobQuery.Where("job_posts.title ILIKE ? OR job_posts.description ILIKE ?", like, like)
	}

	if err := jobQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit

	// We need a struct to scan the extra distance column when location is used
	type jobWithDistance struct {
		models.JobPost
		DistanceKM *float64 `gorm:"column:distance_km" json:"distance_km,omitempty"`
	}

	var jobResults []jobWithDistance

	fetchQuery := jobQuery.
		Preload("CreatedBy").
		Preload("Category").
		Preload("JobMedia")

	if useLocation {
		distCol := fmt.Sprintf(haversineSelect+" AS distance_km", *params.Latitude, *params.Longitude, *params.Latitude)
		fetchQuery = fetchQuery.
			Select("job_posts.*, " + distCol).
			Order("distance_km ASC")
	} else {
		fetchQuery = fetchQuery.
			Select("job_posts.*").
			Order("job_posts.created_at DESC")
	}

	if err := fetchQuery.
		Limit(params.Limit).
		Offset(offset).
		Find(&jobResults).Error; err != nil {
		return nil, 0, err
	}

	// ----- Transform results -----
	jobsArray := make([]JobPostValue, 0, len(jobResults))
	for _, row := range jobResults {
		post := row.JobPost

		mediaList := make([]TypeMedia, 0, len(post.JobMedia))
		for _, m := range post.JobMedia {
			mediaList = append(mediaList, TypeMedia{
				URL:       m.URL,
				MediaType: m.MediaType,
				FileName:  m.FileName,
				FileSize:  m.FileSize,
				Thumbnail: m.Thumbnail,
			})
		}

		creator := TypeUser{
			ID:           post.CreatedBy.ID,
			FirstName:    post.CreatedBy.FirstName,
			LastName:     post.CreatedBy.LastName,
			Email:        post.CreatedBy.Email,
			PhoneNumber:  post.CreatedBy.PhoneNumber,
			ProfilePhoto: post.CreatedBy.ProfilePhoto,
		}
		cat := TypeCategory{
			ID:   post.Category.ID,
			Name: post.Category.Name,
		}
		job := JobPostValue{
			ID:          post.ID,
			Title:       post.Title,
			Description: post.Description,
			Budget:      post.Budget,
			OpenBudget:  post.OpenBudget,
			Address:     post.Address,
			Status:      post.Status,
			CreatedBy:   creator,
			Category:    cat,
			Media:       mediaList,
			CreatedAt:   post.CreatedAt,
			UpdatedAt:   post.UpdatedAt,
			DistanceKM:  row.DistanceKM,
		}

		jobsArray = append(jobsArray, job)
	}

	return jobsArray, total, nil
}
