package repository

import (
	"strings"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	BackgroundJobRunStatusRunning = "running"
	BackgroundJobRunStatusSuccess = "success"
	BackgroundJobRunStatusFailed  = "failed"
)

type BackgroundJobRunRepo struct {
	db *gorm.DB
}

func NewBackgroundJobRunRepo(db *gorm.DB) *BackgroundJobRunRepo {
	return &BackgroundJobRunRepo{db: db}
}

type ListBackgroundJobRunsInput struct {
	JobName string
	Status  string
	Limit   int
	Offset  int
}

func (r *BackgroundJobRunRepo) Create(run *model.BackgroundJobRun) error {
	return r.db.Create(run).Error
}

func (r *BackgroundJobRunRepo) Update(run *model.BackgroundJobRun) error {
	return r.db.Save(run).Error
}

func (r *BackgroundJobRunRepo) Get(id uuid.UUID) (*model.BackgroundJobRun, error) {
	var run model.BackgroundJobRun
	err := r.db.First(&run, "id = ?", id).Error
	return &run, err
}

func (r *BackgroundJobRunRepo) List(input ListBackgroundJobRunsInput) ([]model.BackgroundJobRun, int64, error) {
	limit := input.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	db := r.db.Model(&model.BackgroundJobRun{})
	if jobName := strings.TrimSpace(input.JobName); jobName != "" {
		db = db.Where("job_name = ?", jobName)
	}
	if status := strings.TrimSpace(input.Status); status != "" {
		db = db.Where("status = ?", status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []model.BackgroundJobRun
	if err := db.Order("started_at DESC").Limit(limit).Offset(offset).Find(&runs).Error; err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

func (r *BackgroundJobRunRepo) PruneBefore(cutoff time.Time) (int64, error) {
	res := r.db.Where("started_at < ?", cutoff).Delete(&model.BackgroundJobRun{})
	return res.RowsAffected, res.Error
}
