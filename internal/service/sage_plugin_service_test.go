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
	if err := db.AutoMigrate(&model.SAGEPlugin{}, &model.SAGEPluginVersion{}, &model.SAGEPluginReview{}, &model.SAGEPluginInstallation{}, &model.SAGEPluginPermissionGrant{}, &model.SAGEPluginInvocation{}, &model.SAGEPluginExecutionReport{}, &model.SAGEPluginUsageLedger{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}); err != nil {
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

func TestSAGEPluginServiceEmitsPluginSyncEvents(t *testing.T) {
	svc, db := newSAGETestService(t)
	syncSvc := NewSyncService(repository.NewSyncRepo(db), &captureHub{})
	svc.SetSyncService(syncSvc)
	developerID := uuid.New()
	userID := uuid.New()
	deviceID := "device-a"
	plugin, _ := createApprovedSAGEPlugin(t, svc, developerID)

	inst, err := svc.InstallPlugin(userID, deviceID, plugin.PluginKey, InstallSAGEPluginInput{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := svc.GrantPermission(userID, deviceID, inst.ID, GrantSAGEPermissionInput{PermissionKey: "plugin.api.call"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RevokeGrant(userID, deviceID, grant.ID); err != nil {
		t.Fatal(err)
	}
	regrant, err := svc.GrantPermission(userID, deviceID, inst.ID, GrantSAGEPermissionInput{PermissionKey: "plugin.api.call"})
	if err != nil {
		t.Fatal(err)
	}
	if regrant.ID != grant.ID || regrant.Status != repository.SAGEGrantStatusActive {
		t.Fatalf("expected revoked grant to be reactivated, got %#v", regrant)
	}
	if _, err := svc.SetInstallationStatus(userID, deviceID, inst.ID, repository.SAGEInstallationStatusDisabled); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetInstallationStatus(userID, deviceID, inst.ID, repository.SAGEInstallationStatusActive); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetInstallationStatus(userID, deviceID, inst.ID, repository.SAGEInstallationStatusUninstalled); err != nil {
		t.Fatal(err)
	}

	events, err := syncSvc.GetEventsAfter(userID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"plugin.installed",
		"plugin.permission_granted",
		"plugin.permission_revoked",
		"plugin.permission_granted",
		"plugin.disabled",
		"plugin.enabled",
		"plugin.uninstalled",
	}
	if len(events) != len(expected) {
		t.Fatalf("expected %d sync events, got %d: %#v", len(expected), len(events), events)
	}
	for i, event := range events {
		if event.EventType != expected[i] {
			t.Fatalf("event %d expected %s, got %s", i, expected[i], event.EventType)
		}
		if event.ObjectType != SyncObjectPlugin || event.ObjectID != inst.ID.String() || event.SourceDeviceID != deviceID {
			t.Fatalf("event %d has wrong stable fields: %#v", i, event)
		}
		if event.Payload["installation_id"] != inst.ID.String() || event.Payload["object_type"] != SyncObjectPlugin {
			t.Fatalf("event %d missing client payload fields: %#v", i, event.Payload)
		}
	}
	if events[1].Payload["grant_id"] != grant.ID.String() || events[1].Payload["permission_key"] != "plugin.api.call" {
		t.Fatalf("permission grant payload mismatch: %#v", events[1].Payload)
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
