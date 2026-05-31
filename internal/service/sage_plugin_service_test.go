package service

import (
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSAGETestService(t *testing.T) (*SAGEPluginService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SAGEPlugin{}, &model.SAGEPluginVersion{}, &model.SAGEPluginReview{}, &model.SAGEPluginInstallation{}, &model.SAGEPluginPermissionGrant{}, &model.SAGEPluginInvocation{}, &model.SAGEPluginExecutionReport{}, &model.SAGEPluginUsageLedger{}); err != nil {
		t.Fatal(err)
	}
	return NewSAGEPluginService(repository.NewSAGEPluginRepo(db)), db
}

func createApprovedSAGEPlugin(t *testing.T, svc *SAGEPluginService, developerID uuid.UUID) (*model.SAGEPlugin, *model.SAGEPluginVersion) {
	t.Helper()
	plugin, err := svc.CreatePlugin(developerID, CreateSAGEPluginInput{PluginKey: "com.example.hotel-booking", Name: "Hotel Booking"})
	if err != nil {
		t.Fatal(err)
	}
	version, _, err := svc.SubmitVersion(developerID, plugin.ID, SubmitSAGEPluginVersionInput{Manifest: validSAGEManifestFixture()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReviewPlugin(uuid.New(), plugin.ID, version.ID, ReviewSAGEPluginInput{Decision: "approved", Reason: "ok"}); err != nil {
		t.Fatal(err)
	}
	plugin, _ = svc.repo.GetPlugin(plugin.ID)
	version, _ = svc.repo.GetVersion(version.ID)
	return plugin, version
}

func TestSAGEPluginServiceSubmitVersionEntersReview(t *testing.T) {
	svc, _ := newSAGETestService(t)
	developerID := uuid.New()
	plugin, err := svc.CreatePlugin(developerID, CreateSAGEPluginInput{PluginKey: "com.example.hotel-booking", Name: "Hotel Booking"})
	if err != nil {
		t.Fatal(err)
	}
	version, validation, err := svc.SubmitVersion(developerID, plugin.ID, SubmitSAGEPluginVersionInput{Manifest: validSAGEManifestFixture()})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid {
		t.Fatalf("expected valid manifest: %v", validation.Errors)
	}
	if version.Status != repository.SAGEVersionStatusSubmitted {
		t.Fatalf("expected submitted, got %s", version.Status)
	}
}

func TestSAGEPluginServiceApprovePublishesCatalog(t *testing.T) {
	svc, _ := newSAGETestService(t)
	createApprovedSAGEPlugin(t, svc, uuid.New())
	page, err := svc.SearchCatalog("hotel", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("expected one catalog item, got %d", page.Total)
	}
}

func TestSAGEPluginServiceCannotInstallUnapprovedPlugin(t *testing.T) {
	svc, _ := newSAGETestService(t)
	developerID := uuid.New()
	_, err := svc.CreatePlugin(developerID, CreateSAGEPluginInput{PluginKey: "com.example.hotel-booking", Name: "Hotel Booking"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.InstallPlugin(uuid.New(), "device", "com.example.hotel-booking", InstallSAGEPluginInput{})
	if err == nil {
		t.Fatal("expected install unapproved plugin to fail")
	}
}

func TestSAGEPluginServicePolicyBundleAndReport(t *testing.T) {
	svc, _ := newSAGETestService(t)
	developerID := uuid.New()
	userID := uuid.New()
	plugin, _ := createApprovedSAGEPlugin(t, svc, developerID)
	inst, err := svc.InstallPlugin(userID, "device-a", plugin.PluginKey, InstallSAGEPluginInput{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := svc.GrantPermission(userID, "device-a", inst.ID, GrantSAGEPermissionInput{PermissionKey: "plugin.api.call"})
	if err != nil {
		t.Fatal(err)
	}
	if grant.PermissionKey != "plugin.api.call" {
		t.Fatal("grant mismatch")
	}
	bundle, err := svc.PolicyBundle(userID, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.GrantedPermissions) != 1 {
		t.Fatalf("expected one grant, got %d", len(bundle.GrantedPermissions))
	}
	inv, err := svc.CreateInvocation(userID, "device-a", CreateSAGEInvocationInput{PluginKey: plugin.PluginKey, ClientRequestID: "req-1", UserIntent: "find hotels"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.CreateInvocation(userID, "device-a", CreateSAGEInvocationInput{PluginKey: plugin.PluginKey, ClientRequestID: "req-1"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.ID != again.ID {
		t.Fatal("expected idempotent invocation")
	}
	report, err := svc.SubmitReport(userID, inv.ID, SubmitSAGEExecutionReportInput{ClientReportID: "report-1", Status: "completed", TokensUsed: 10})
	if err != nil {
		t.Fatal(err)
	}
	reportAgain, err := svc.SubmitReport(userID, inv.ID, SubmitSAGEExecutionReportInput{ClientReportID: "report-1", Status: "completed", TokensUsed: 10})
	if err != nil {
		t.Fatal(err)
	}
	if report.ID != reportAgain.ID {
		t.Fatal("expected idempotent report")
	}
	metrics, err := svc.DeveloperMetrics(developerID, plugin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Invocations != 1 || metrics.Completed != 1 || metrics.TokensUsed != 10 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}
