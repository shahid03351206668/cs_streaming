package job

import "tasksy/models"

func (s *Service) GetJobFeed(category, searchQuery string, page, limit int) ([]JobPostValue, int64, error) {
	var jobResults []models.JobPost
	var total int64

	jobQuery := s.db.Model(&models.JobPost{}).Where("status = ?", models.JobStatusOpen)

	if category != "" {
		jobQuery = jobQuery.Where("category_id = ?", category)
	}

	if searchQuery != "" {
		jobQuery = jobQuery.Where("title ILIKE ? OR description ILIKE ?", "%"+searchQuery+"%", "%"+searchQuery+"%")
	}

	if err := jobQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := jobQuery.
		Preload("CreatedBy").
		Preload("Category").
		Preload("JobMedia").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&jobResults).Error; err != nil {
		return nil, 0, err
	}

	jobsArray := make([]JobPostValue, 0, len(jobResults))
	for _, post := range jobResults {
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

		jobsArray = append(jobsArray, JobPostValue{
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
		})
	}

	return jobsArray, total, nil
}


