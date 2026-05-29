package service

import (
	"context"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/config"
	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPhase1SmokeAuthConversationOfflineQRAndSync(t *testing.T) {
	env := newPhase1SmokeEnv(t)

	alice := env.verifyNewUser(t, "alice-device-1", "alice-pubkey")
	bob := env.verifyNewUser(t, "bob-device-1", "bob-pubkey")

	conv, err := env.convSvc.CreateConversation(alice.User.ID, &CreateConversationRequest{
		Type:           "private",
		ParticipantIDs: []uuid.UUID{bob.User.ID},
	})
	if err != nil {
		t.Fatalf("create private conversation: %v", err)
	}

	delivery, err := env.msgSvc.SendMessage(alice.User.ID, &SendMessageRequest{
		ConversationID: conv.ID,
		Content:        "hello phase1",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if len(delivery.Participants) != 2 {
		t.Fatalf("expected two delivery participants, got %+v", delivery.Participants)
	}

	if err := env.msgSvc.SaveOfflineMessages(delivery.Message, delivery.Participants, map[string]bool{"alice-device-1": true}); err != nil {
		t.Fatalf("save offline messages: %v", err)
	}
	bobOffline, err := env.msgSvc.FetchOfflineMessages(bob.User.ID, "bob-device-1")
	if err != nil {
		t.Fatalf("fetch bob offline messages: %v", err)
	}
	if len(bobOffline) != 1 || bobOffline[0].MessageID != delivery.Message.ID {
		t.Fatalf("expected bob offline message, got %+v", bobOffline)
	}
	if err := env.msgSvc.AckOfflineMessages([]uuid.UUID{bobOffline[0].ID}); err != nil {
		t.Fatalf("ack bob offline message: %v", err)
	}
	bobOffline, err = env.msgSvc.FetchOfflineMessages(bob.User.ID, "bob-device-1")
	if err != nil {
		t.Fatalf("fetch bob offline after ack: %v", err)
	}
	if len(bobOffline) != 0 {
		t.Fatalf("expected no bob offline messages after ack, got %+v", bobOffline)
	}

	bobEvents, err := env.syncSvc.GetEvents(bob.User.ID, "bob-device-1", 100)
	if err != nil {
		t.Fatalf("fetch bob sync events: %v", err)
	}
	if len(bobEvents) != 1 || bobEvents[0].EventType != "message.created" || bobEvents[0].Payload["message_id"] != delivery.Message.ID.String() {
		t.Fatalf("expected bob message.created sync event, got %+v", bobEvents)
	}
	if err := env.syncSvc.AckEvents(bob.User.ID, "bob-device-1", bobEvents[0].Sequence); err != nil {
		t.Fatalf("ack bob sync event: %v", err)
	}
	bobEvents, err = env.syncSvc.GetEvents(bob.User.ID, "bob-device-1", 100)
	if err != nil {
		t.Fatalf("fetch bob sync after ack: %v", err)
	}
	if len(bobEvents) != 0 {
		t.Fatalf("expected no bob sync events after ack, got %+v", bobEvents)
	}

	aliceEvents, err := env.syncSvc.GetEvents(alice.User.ID, "alice-device-2", 100)
	if err != nil {
		t.Fatalf("fetch alice sync events: %v", err)
	}
	if len(aliceEvents) != 1 || aliceEvents[0].EventType != "message.created" {
		t.Fatalf("expected sender cross-device sync event, got %+v", aliceEvents)
	}

	start, err := env.pairingSvc.StartPairing(alice.User.ID, "alice-device-1")
	if err != nil {
		t.Fatalf("start QR pairing: %v", err)
	}
	pairedDevice, err := env.pairingSvc.ClaimPairing(&ClaimPairingRequest{
		QRPayload:       start.QRPayload,
		NewDeviceID:     "alice-device-2",
		NewDeviceName:   "Alice Tablet",
		NewDevicePubKey: "alice-device-2-pubkey",
		Signature:       "valid-signature",
	})
	if err != nil {
		t.Fatalf("claim QR pairing: %v", err)
	}
	if pairedDevice.UserID != alice.User.ID || pairedDevice.DeviceID != "alice-device-2" {
		t.Fatalf("unexpected paired device: %+v", pairedDevice)
	}
}

type phase1SmokeVerifier struct{}

func (v *phase1SmokeVerifier) VerifyEd25519Challenge(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

type phase1SmokeEnv struct {
	identitySvc *IdentityService
	convSvc     *ConversationService
	msgSvc      *MessageService
	syncSvc     *SyncService
	pairingSvc  *DevicePairingService
}

func newPhase1SmokeEnv(t *testing.T) *phase1SmokeEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.AuthChallenge{}, &model.AdmissionRequest{}, &model.ServerAdmission{}, &model.DevicePairingSession{}, &model.Conversation{}, &model.ConversationParticipant{}, &model.Message{}, &model.OfflineMessage{}, &model.SyncEvent{}, &model.SyncCursor{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	admissionSvc := NewAdmissionServiceWithRepo(userRepo, repository.NewAdmissionRepo(db), config.AdmissionConfig{PolicyType: "protocol"})
	verifier := &phase1SmokeVerifier{}
	identitySvc := NewIdentityServiceWithAdmission(userRepo, config.JWTConfig{
		Secret:          "test-secret-that-is-long-enough-for-phase1-smoke",
		AccessTokenMins: 60,
		Issuer:          "agent-os-test",
	}, verifier, admissionSvc)
	syncSvc := NewSyncService(repository.NewSyncRepo(db), &captureHub{})
	msgSvc := NewMessageService(convRepo, userRepo)
	msgSvc.SetSyncService(syncSvc)
	return &phase1SmokeEnv{
		identitySvc: identitySvc,
		convSvc:     NewConversationService(convRepo, userRepo),
		msgSvc:      msgSvc,
		syncSvc:     syncSvc,
		pairingSvc:  NewDevicePairingService(repository.NewDevicePairingRepo(db), userRepo, verifier),
	}
}

func (e *phase1SmokeEnv) verifyNewUser(t *testing.T, deviceID, pubKey string) *AuthResponse {
	t.Helper()
	challenge, err := e.identitySvc.InitiateChallenge(deviceID, pubKey)
	if err != nil {
		t.Fatalf("initiate challenge for %s: %v", pubKey, err)
	}
	res, err := e.identitySvc.VerifySignature(&VerifyRequest{
		DeviceID:   deviceID,
		UserPubKey: pubKey,
		Nonce:      challenge.Nonce,
		Signature:  "valid-signature",
	})
	if err != nil {
		t.Fatalf("verify signature for %s: %v", pubKey, err)
	}
	if res.AccessToken == "" || res.User == nil || res.Device == nil || !res.IsNewUser {
		t.Fatalf("unexpected auth response for %s: %+v", pubKey, res)
	}
	return res
}
