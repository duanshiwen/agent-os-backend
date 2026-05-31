package service

import (
	"context"
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

type OpsService struct {
	backgroundTasks *BackgroundTasks
	jobRunRepo      *repository.BackgroundJobRunRepo
}

func NewOpsService(backgroundTasks *BackgroundTasks, jobRunRepo *repository.BackgroundJobRunRepo) *OpsService {
	return &OpsService{backgroundTasks: backgroundTasks, jobRunRepo: jobRunRepo}
}

type ListBackgroundJobRunsInput struct {
	JobName string `json:"job_name"`
	Status  string `json:"status"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

type BackgroundJobRunPage struct {
	Items         []model.BackgroundJobRun `json:"items"`
	Limit, Offset int                      `json:"limit"`
	Total         int64                    `json:"total"`
}

type RunBackgroundJobsResult struct {
	Results map[string]string `json:"results"`
}

func (s *OpsService) ListBackgroundJobRuns(input ListBackgroundJobRunsInput) (*BackgroundJobRunPage, error) {
	if s.jobRunRepo == nil {
		return &BackgroundJobRunPage{Items: []model.BackgroundJobRun{}, Limit: normalizedLimit(input.Limit), Offset: normalizedOffset(input.Offset), Total: 0}, nil
	}
	items, total, err := s.jobRunRepo.List(repository.ListBackgroundJobRunsInput{JobName: input.JobName, Status: input.Status, Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, err
	}
	return &BackgroundJobRunPage{Items: items, Limit: normalizedLimit(input.Limit), Offset: normalizedOffset(input.Offset), Total: total}, nil
}

func (s *OpsService) GetBackgroundJobRun(id uuid.UUID) (*model.BackgroundJobRun, error) {
	return s.jobRunRepo.Get(id)
}

func (s *OpsService) RunBackgroundJobsOnce(ctx context.Context) (*RunBackgroundJobsResult, error) {
	if s.backgroundTasks == nil {
		return &RunBackgroundJobsResult{Results: map[string]string{}}, nil
	}
	errs := s.backgroundTasks.RunOnce(ctx)
	results := map[string]string{}
	for job, err := range errs {
		if err != nil {
			results[job] = "failed: " + err.Error()
		} else {
			results[job] = "success"
		}
	}
	return &RunBackgroundJobsResult{Results: results}, nil
}

func normalizedLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}

func normalizedOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func IsBackgroundJobRunStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case "", repository.BackgroundJobRunStatusRunning, repository.BackgroundJobRunStatusSuccess, repository.BackgroundJobRunStatusFailed:
		return true
	default:
		return false
	}
}
