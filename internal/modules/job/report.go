package job

import (
	"errors"
	"fmt"

	"tasksy/models"
)

var ErrAlreadyReported = errors.New("you have already reported this job")

func (s *Service) ReportJob(reporterID, jobPostID, reason, details string) (*models.JobReport, error) {
	var job models.JobPost
	if err := s.db.First(&job, "id = ?", jobPostID).Error; err != nil {
		return nil, ErrJobNotFound
	}

	var existing models.JobReport
	if err := s.db.Where("job_post_id = ? AND reporter_id = ?", jobPostID, reporterID).First(&existing).Error; err == nil {
		return nil, ErrAlreadyReported
	}

	report := models.JobReport{
		JobPostID:  jobPostID,
		ReporterID: reporterID,
		Reason:     reason,
		Details:    details,
	}

	if err := s.db.Create(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to create job report: %w", err)
	}

	return &report, nil
}
