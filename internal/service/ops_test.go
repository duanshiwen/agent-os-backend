package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOpsServiceListsBackgroundJobRuns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.BackgroundJobRun{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewBackgroundJobRunRepo(db)
	started := time.Now().UTC()
	if err := repo.Create(&model.BackgroundJobRun{JobName: BackgroundJobSyncEventCleanup, Trigger: "manual", Status: repository.BackgroundJobRunStatusSuccess, StartedAt: started}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.Create(&model.BackgroundJobRun{JobName: BackgroundJobOfflineMessageCleanup, Trigger: "manual", Status: repository.BackgroundJobRunStatusFailed, StartedAt: started.Add(-time.Minute), ErrorMessage: "boom"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	svc := NewOpsService(nil, repo)
	page, err := svc.ListBackgroundJobRuns(ListBackgroundJobRunsInput{Status: repository.BackgroundJobRunStatusFailed, Limit: 10})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ErrorMessage != "boom" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestOpsServiceRunBackgroundJobsOnceHandlesNilTasks(t *testing.T) {
	svc := NewOpsService(nil, nil)
	result, err := svc.RunBackgroundJobsOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("expected empty results, got %+v", result)
	}
}
