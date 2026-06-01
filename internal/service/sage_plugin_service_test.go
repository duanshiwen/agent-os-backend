package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-os/backend/internal/config"
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
	if err := db.AutoMigrate(&model.ObjectRecord{}, &model.SAGEPlugin{}, &model.SAGEPluginVersion{}, &model.SAGEPluginReview{}, &model.SAGEPluginInstallation{}, &model.SAGEPluginPermissionGrant{}, &model.SAGEPluginInvocation{}, &model.SAGEPluginExecutionReport{}, &model.SAGEPluginUsageLedger{}, &model.SyncEvent{}, &model.SyncCursor{}, &model.SyncSequence{}, &model.CapabilityDefinition{}, &model.PolicyRule{}, &model.PolicyDecision{}, &model.ApprovalReceipt{}, &model.KillSwitch{}, &model.GovernanceScanResult{}); err != nil {
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

func TestSAGEPluginServiceBindsActiveOwnedIconAndPackageAssets(t *testing.T) {
	svc, db := newSAGETestService(t)
	objectSvc := NewObjectService(repository.NewObjectRecordsRepo(db), &fakeObjectStorageBackend{}, config.ObjectStorageConfig{Bucket: "agentos-test", UploadTTLSecs: 900, DownloadTTLSecs: 900})
	svc.SetObjectService(objectSvc)
	developerID := uuid.New()
	icon, err := objectSvc.StoreObject(context.Background(), developerID, StoreObjectInput{Scope: "sage-plugin-icons", Filename: "hotel.png", ContentType: "image/png", Content: []byte("png")})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := objectSvc.StoreObject(context.Background(), developerID, StoreObjectInput{Scope: "sage-plugin-packages", Filename: "hotel.zip", ContentType: "application/zip", Content: []byte("zip")})
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := svc.CreatePlugin(developerID, CreateSAGEPluginInput{PluginKey: "com.example.hotel-booking", Name: "Hotel Booking", IconObjectID: &icon.ID})
	if err != nil {
		t.Fatal(err)
	}
	if plugin.IconObjectID == nil || *plugin.IconObjectID != icon.ID {
		t.Fatalf("expected icon object binding, got %#v", plugin.IconObjectID)
	}
	version, _, err := svc.SubmitVersion(developerID, plugin.ID, SubmitSAGEPluginVersionInput{Manifest: validSAGEManifestFixture(), PackageObjectID: &pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	if version.PackageObjectID == nil || *version.PackageObjectID != pkg.ID {
		t.Fatalf("expected package object binding, got %#v", version.PackageObjectID)
	}
	if _, err := svc.ReviewPlugin(uuid.New(), plugin.ID, version.ID, ReviewSAGEPluginInput{Decision: "approved", Reason: "ok"}); err != nil {
		t.Fatal(err)
	}
	catalog, err := svc.GetCatalogPlugin(plugin.PluginKey)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.IconAsset == nil || catalog.IconAsset.ObjectID != icon.ID || catalog.IconAsset.ContentType != "image/png" {
		t.Fatalf("expected safe icon metadata, got %#v", catalog.IconAsset)
	}
	if catalog.PackageAsset == nil || catalog.PackageAsset.ObjectID != pkg.ID || catalog.PackageAsset.ContentType != "application/zip" {
		t.Fatalf("expected safe package metadata, got %#v", catalog.PackageAsset)
	}
}

func TestSAGEPluginServiceRejectsInvalidAssetBindings(t *testing.T) {
	svc, db := newSAGETestService(t)
	objectSvc := NewObjectService(repository.NewObjectRecordsRepo(db), &fakeObjectStorageBackend{}, config.ObjectStorageConfig{Bucket: "agentos-test", UploadTTLSecs: 900, DownloadTTLSecs: 900})
	svc.SetObjectService(objectSvc)
	developerID := uuid.New()
	otherDeveloperID := uuid.New()
	otherIcon, err := objectSvc.StoreObject(context.Background(), otherDeveloperID, StoreObjectInput{Scope: "sage-plugin-icons", Filename: "other.png", ContentType: "image/png", Content: []byte("png")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePlugin(developerID, CreateSAGEPluginInput{PluginKey: "com.example.bad-icon", Name: "Bad Icon", IconObjectID: &otherIcon.ID}); err == nil || !strings.Contains(err.Error(), ErrObjectNotFound.Error()) {
		t.Fatalf("expected cross-owner icon rejection, got %v", err)
	}
	textObject, err := objectSvc.StoreObject(context.Background(), developerID, StoreObjectInput{Scope: "sage-plugin-packages", Filename: "package.txt", ContentType: "text/plain", Content: []byte("text")})
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := svc.CreatePlugin(developerID, CreateSAGEPluginInput{PluginKey: "com.example.bad-package", Name: "Bad Package"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.SubmitVersion(developerID, plugin.ID, SubmitSAGEPluginVersionInput{Manifest: validSAGEManifestFixture(), PackageObjectID: &textObject.ID}); err == nil || !strings.Contains(err.Error(), "unsupported package content type") {
		t.Fatalf("expected package content type rejection, got %v", err)
	}
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

func TestSAGEPluginServiceClientRuntimeContract(t *testing.T) {
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
	if bundle.PluginKey != plugin.PluginKey || bundle.Version == "" || bundle.PolicyBundleVersion == "" {
		t.Fatalf("policy bundle missing stable identity fields: %#v", bundle)
	}
	if len(bundle.GrantedPermissions) != 1 || bundle.GrantedPermissions[0]["key"] != "plugin.api.call" || bundle.GrantedPermissions[0]["risk"] != SAGERiskLow {
		t.Fatalf("expected low-risk plugin api grant in policy bundle, got %#v", bundle.GrantedPermissions)
	}
	if len(bundle.DeniedPermissions) != 1 || bundle.DeniedPermissions[0] != "transaction.booking.create" {
		t.Fatalf("expected manifest high-risk booking permission to remain denied, got %#v", bundle.DeniedPermissions)
	}
	if len(bundle.RuntimeGuards) != 1 || bundle.RuntimeGuards[0]["decision"] != "require_user_confirmation" {
		t.Fatalf("expected runtime guard for high-risk denied permission, got %#v", bundle.RuntimeGuards)
	}
	if bundle.Reporting["required"] != true || bundle.Reporting["endpoint"] != "/api/v1/sage/invocations/{id}/reports" {
		t.Fatalf("expected stable reporting contract, got %#v", bundle.Reporting)
	}

	inv, err := svc.CreateInvocation(userID, "device-a", CreateSAGEInvocationInput{PluginKey: plugin.PluginKey, InstallationID: &inst.ID, ClientRequestID: "req-1", UserIntent: "find hotels", FlowID: "flow-1", FlowHash: "flow-hash-1", RiskLevel: SAGERiskLow, PermissionsUsed: []string{"plugin.api.call"}, PolicyDecision: map[string]any{"decision": GovernanceDecisionAllow}})
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
	if inv.InstallationID == nil || *inv.InstallationID != inst.ID || inv.PolicyDecision["decision"] != GovernanceDecisionAllow {
		t.Fatalf("invocation did not preserve client runtime context: %#v", inv)
	}

	report, err := svc.SubmitReport(userID, inv.ID, SubmitSAGEExecutionReportInput{ClientReportID: "report-1", Status: "completed", FlowID: "flow-1", StepsCompleted: []string{"search_hotels", "present_options"}, StepSummaries: map[string]any{"search_hotels": "mock candidates returned"}, TokensUsed: 10, Metering: map[string]any{"unit": "tokens"}})
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
	if report.Status != repository.SAGEInvocationStatusCompleted || report.TokensUsed != 10 || len(report.StepsCompleted) != 2 {
		t.Fatalf("execution report did not preserve client runtime summary: %#v", report)
	}
	metrics, err := svc.DeveloperMetrics(developerID, plugin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Invocations != 1 || metrics.Completed != 1 || metrics.TokensUsed != 10 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestSAGEPluginServiceGovernanceEnforceClientRuntimeOutcomes(t *testing.T) {
	svc, db := newSAGETestService(t)
	governance := NewGovernanceService(repository.NewGovernanceRepo(db))
	svc.SetGovernanceEnforcer(NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{SAGEMode: GovernanceEnforcementModeEnforce}))

	developerID := uuid.New()
	userID := uuid.New()
	plugin, _ := createApprovedSAGEPlugin(t, svc, developerID)
	inst, err := svc.InstallPlugin(userID, "device-a", plugin.PluginKey, InstallSAGEPluginInput{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{Name: "Deny SAGE invocation", CapabilityKey: "plugin.api.call", SubjectType: GovernanceSubjectSAGEInvocation, Effect: GovernanceDecisionDeny, Priority: 1, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create deny rule: %v", err)
	}
	_, err = svc.CreateInvocation(userID, "device-a", CreateSAGEInvocationInput{PluginKey: plugin.PluginKey, ClientRequestID: "deny-req", RiskLevel: SAGERiskLow, PermissionsUsed: []string{"plugin.api.call"}})
	assertSAGEGovernanceError(t, err, ErrGovernanceDenied, GovernanceDecisionDeny, GovernanceSubjectSAGEInvocation, plugin.ID.String(), "plugin.api.call")

	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{Name: "Require approval SAGE grant", CapabilityKey: "third_party.api.call", SubjectType: GovernanceSubjectSAGEPermissionGrant, Effect: GovernanceDecisionRequireUserApproval, Priority: 10, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create approval rule: %v", err)
	}
	_, err = svc.GrantPermission(userID, "device-a", inst.ID, GrantSAGEPermissionInput{PermissionKey: "third_party.api.call"})
	enforcementErr := assertSAGEGovernanceError(t, err, ErrGovernanceApprovalRequired, GovernanceDecisionRequireUserApproval, GovernanceSubjectSAGEPermissionGrant, inst.ID.String(), "third_party.api.call")
	if enforcementErr.Result == nil || enforcementErr.Result.ApprovalReceipt == nil || enforcementErr.Result.ApprovalToken == "" {
		t.Fatalf("expected approval-required error to include receipt and token, got %#v", enforcementErr.Result)
	}

	_, err = svc.GrantPermission(userID, "device-a", inst.ID, GrantSAGEPermissionInput{PermissionKey: "third_party.api.call", ApprovalToken: "not-a-valid-token"})
	assertSAGEGovernanceError(t, err, ErrGovernanceApprovalInvalid, "", GovernanceSubjectSAGEPermissionGrant, inst.ID.String(), "third_party.api.call")

	grant, err := svc.GrantPermission(userID, "device-a", inst.ID, GrantSAGEPermissionInput{PermissionKey: "third_party.api.call", ApprovalToken: enforcementErr.Result.ApprovalToken})
	if err != nil {
		t.Fatalf("expected valid approval token to allow grant: %v", err)
	}
	if grant.PermissionKey != "third_party.api.call" || grant.Status != repository.SAGEGrantStatusActive {
		t.Fatalf("unexpected approved grant: %#v", grant)
	}
}

func assertSAGEGovernanceError(t *testing.T, err error, expectedCause error, expectedDecision, expectedSubjectType, expectedSubjectID, expectedCapability string) *GovernanceEnforcementError {
	t.Helper()
	if !errors.Is(err, expectedCause) {
		t.Fatalf("expected %v, got %v", expectedCause, err)
	}
	var enforcementErr *GovernanceEnforcementError
	if !errors.As(err, &enforcementErr) {
		t.Fatalf("expected GovernanceEnforcementError, got %T %v", err, err)
	}
	if enforcementErr.Input.SubjectType != expectedSubjectType || enforcementErr.Input.SubjectID != expectedSubjectID || enforcementErr.Input.CapabilityKey != expectedCapability {
		t.Fatalf("unexpected governance input details: %#v", enforcementErr.Input)
	}
	if expectedDecision != "" {
		if enforcementErr.Result == nil || enforcementErr.Result.Decision == nil || enforcementErr.Result.Decision.Decision != expectedDecision {
			t.Fatalf("unexpected governance decision: %#v", enforcementErr.Result)
		}
	}
	return enforcementErr
}

func TestSAGEPluginServiceGovernanceObserveRecordsGrantAndInvocationDecisions(t *testing.T) {
	svc, db := newSAGETestService(t)
	governance := NewGovernanceService(repository.NewGovernanceRepo(db))
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{Name: "Observe deny SAGE grant", CapabilityKey: "plugin.api.call", SubjectType: GovernanceSubjectSAGEPermissionGrant, Effect: GovernanceDecisionDeny, Priority: 1, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create grant rule: %v", err)
	}
	if _, err := governance.CreatePolicyRule(CreatePolicyRuleInput{Name: "Observe deny SAGE invocation", CapabilityKey: "sage.invocation.create", SubjectType: GovernanceSubjectSAGEInvocation, Effect: GovernanceDecisionDeny, Priority: 1, Status: GovernanceStatusActive}); err != nil {
		t.Fatalf("create invocation rule: %v", err)
	}
	svc.SetGovernanceEnforcer(NewGovernanceEnforcer(governance, GovernanceEnforcementConfig{}))
	developerID := uuid.New()
	userID := uuid.New()
	plugin, _ := createApprovedSAGEPlugin(t, svc, developerID)
	inst, err := svc.InstallPlugin(userID, "device-a", plugin.PluginKey, InstallSAGEPluginInput{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	grant, err := svc.GrantPermission(userID, "device-a", inst.ID, GrantSAGEPermissionInput{PermissionKey: "plugin.api.call"})
	if err != nil {
		t.Fatalf("observe mode should not block grant: %v", err)
	}
	if grant.Status != repository.SAGEGrantStatusActive {
		t.Fatalf("expected active grant, got %#v", grant)
	}
	inv, err := svc.CreateInvocation(userID, "device-a", CreateSAGEInvocationInput{PluginKey: plugin.PluginKey, ClientRequestID: "governance-req-1", UserIntent: "find hotels"})
	if err != nil {
		t.Fatalf("observe mode should not block invocation: %v", err)
	}
	if inv.PolicyDecision["mode"] != GovernanceEnforcementModeObserve || inv.PolicyDecision["would_have_blocked"] != true {
		t.Fatalf("expected invocation to carry observe policy hint, got %#v", inv.PolicyDecision)
	}
}
