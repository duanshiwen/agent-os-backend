package service

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDevicePairingServiceStartPairingCreatesQRSession(t *testing.T) {
	svc, pairingRepo, user := newDevicePairingTestService(t)

	res, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}
	if res.PairingSessionID == uuid.Nil {
		t.Fatal("expected pairing session id")
	}
	if res.QRPayload == "" {
		t.Fatal("expected qr payload")
	}
	if res.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("expected future expiry, got %d", res.ExpiresAt)
	}

	payload := decodeQRPayloadForTest(t, res.QRPayload)
	if payload.Version != 1 || payload.ServerID != "default" || payload.PairingSessionID != res.PairingSessionID {
		t.Fatalf("unexpected qr payload: %+v", payload)
	}
	if payload.PairingToken == "" {
		t.Fatal("expected pairing token in qr payload")
	}

	session, err := pairingRepo.GetPairingSession(res.PairingSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.UserID != user.ID || session.CreatedByDeviceID != "old-device-1" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if session.PairingTokenHash == "" {
		t.Fatal("expected pairing token hash")
	}
	if session.PairingTokenHash == payload.PairingToken || strings.Contains(session.PairingTokenHash, payload.PairingToken) {
		t.Fatalf("pairing token stored in plaintext: hash=%q token=%q", session.PairingTokenHash, payload.PairingToken)
	}
	if session.QRPayloadHash == "" || session.UsedAt != nil {
		t.Fatalf("unexpected qr hash/used state: %+v", session)
	}
}

func TestDevicePairingServiceStartPairingRejectsUnknownOldDevice(t *testing.T) {
	svc, _, user := newDevicePairingTestService(t)

	_, err := svc.StartPairing(user.ID, "missing-device")
	if err == nil || !strings.Contains(err.Error(), "device not found") {
		t.Fatalf("expected device not found error, got %v", err)
	}
}

func TestDevicePairingServiceStartPairingRejectsDeviceFromAnotherUser(t *testing.T) {
	svc, _, _ := newDevicePairingTestService(t)

	_, err := svc.StartPairing(uuid.New(), "old-device-1")
	if err == nil || !strings.Contains(err.Error(), "device does not belong to user") {
		t.Fatalf("expected ownership error, got %v", err)
	}
}

func TestDevicePairingServiceClaimPairingCreatesNewDevice(t *testing.T) {
	svc, pairingRepo, user := newDevicePairingTestService(t)
	verifier := svc.verifier.(*stubVerifier)
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}

	device, err := svc.ClaimPairing(&ClaimPairingRequest{
		QRPayload:       start.QRPayload,
		NewDeviceID:     "new-device-1",
		NewDeviceName:   "New Device",
		NewDevicePubKey: "new-pubkey",
		Signature:       "valid-signature",
	})
	if err != nil {
		t.Fatalf("claim pairing: %v", err)
	}
	if verifier.calls != 1 {
		t.Fatalf("expected verifier to be called once, got %d", verifier.calls)
	}
	if verifier.lastPublicKey != "new-pubkey" || verifier.lastSignature != "valid-signature" {
		t.Fatalf("verifier called with wrong key/signature: key=%q sig=%q", verifier.lastPublicKey, verifier.lastSignature)
	}
	if !strings.Contains(verifier.lastChallenge, "new-device-1") || !strings.Contains(verifier.lastChallenge, "new-pubkey") {
		t.Fatalf("canonical claim challenge should bind device id and pubkey, got %q", verifier.lastChallenge)
	}
	if device.UserID != user.ID || device.DeviceID != "new-device-1" || device.DevicePubKey != "new-pubkey" {
		t.Fatalf("unexpected device: %+v", device)
	}

	session, err := pairingRepo.GetPairingSession(start.PairingSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.UsedAt == nil || session.ClaimedByDeviceID != "new-device-1" {
		t.Fatalf("expected session marked used by new device, got %+v", session)
	}
}

func TestDevicePairingServiceClaimPairingRejectsInvalidNewDeviceSignature(t *testing.T) {
	svc, _, user := newDevicePairingTestService(t)
	svc.verifier.(*stubVerifier).valid = false
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}

	_, err = svc.ClaimPairing(&ClaimPairingRequest{
		QRPayload:       start.QRPayload,
		NewDeviceID:     "new-device-1",
		NewDevicePubKey: "new-pubkey",
		Signature:       "invalid-signature",
	})
	if err == nil || !strings.Contains(err.Error(), "new device signature verification failed") {
		t.Fatalf("expected signature verification failure, got %v", err)
	}
}

func TestDevicePairingServiceClaimPairingRejectsExpiredPayload(t *testing.T) {
	svc, pairingRepo, user := newDevicePairingTestService(t)
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}
	session, err := pairingRepo.GetPairingSession(start.PairingSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.ExpiresAt = time.Now().Add(-time.Minute)
	if err := pairingRepo.SavePairingSession(session); err != nil {
		t.Fatalf("save expired session: %v", err)
	}

	_, err = svc.ClaimPairing(&ClaimPairingRequest{QRPayload: start.QRPayload, NewDeviceID: "new-device-1", NewDevicePubKey: "new-pubkey", Signature: "valid-signature"})
	if err == nil || !strings.Contains(err.Error(), "pairing session expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestDevicePairingServiceClaimPairingRejectsUsedPayload(t *testing.T) {
	svc, _, user := newDevicePairingTestService(t)
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}
	if _, err := svc.ClaimPairing(&ClaimPairingRequest{QRPayload: start.QRPayload, NewDeviceID: "new-device-1", NewDevicePubKey: "new-pubkey", Signature: "valid-signature"}); err != nil {
		t.Fatalf("first claim pairing: %v", err)
	}

	_, err = svc.ClaimPairing(&ClaimPairingRequest{QRPayload: start.QRPayload, NewDeviceID: "new-device-2", NewDevicePubKey: "new-pubkey-2", Signature: "valid-signature"})
	if err == nil || !strings.Contains(err.Error(), "pairing session already used") {
		t.Fatalf("expected used error, got %v", err)
	}
}

func TestDevicePairingServiceClaimPairingRejectsTamperedToken(t *testing.T) {
	svc, pairingRepo, user := newDevicePairingTestService(t)
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}
	payload := decodeQRPayloadForTest(t, start.QRPayload)
	payload.PairingToken = "tampered-token"
	tamperedPayload, err := encodeQRPayload(payload)
	if err != nil {
		t.Fatalf("encode tampered payload: %v", err)
	}
	session, err := pairingRepo.GetPairingSession(start.PairingSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.QRPayloadHash = sha256Hex(tamperedPayload)
	if err := pairingRepo.SavePairingSession(session); err != nil {
		t.Fatalf("save tampered qr hash: %v", err)
	}

	_, err = svc.ClaimPairing(&ClaimPairingRequest{QRPayload: tamperedPayload, NewDeviceID: "new-device-1", NewDevicePubKey: "new-pubkey", Signature: "valid-signature"})
	if err == nil || !strings.Contains(err.Error(), "invalid pairing token") {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func TestDevicePairingServiceClaimPairingRejectsDuplicateDeviceID(t *testing.T) {
	svc, _, user := newDevicePairingTestService(t)
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}

	_, err = svc.ClaimPairing(&ClaimPairingRequest{QRPayload: start.QRPayload, NewDeviceID: "old-device-1", NewDevicePubKey: "new-pubkey", Signature: "valid-signature"})
	if err == nil || !strings.Contains(err.Error(), "device id already exists") {
		t.Fatalf("expected duplicate device error, got %v", err)
	}
}

func TestDevicePairingServiceClaimPairingAllowsOnlyOneConcurrentClaim(t *testing.T) {
	svc, pairingRepo, user := newDevicePairingTestService(t)
	start, err := svc.StartPairing(user.ID, "old-device-1")
	if err != nil {
		t.Fatalf("start pairing: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, deviceID := range []string{"new-device-1", "new-device-2"} {
		deviceID := deviceID
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.ClaimPairing(&ClaimPairingRequest{
				QRPayload:       start.QRPayload,
				NewDeviceID:     deviceID,
				NewDevicePubKey: deviceID + "-pubkey",
				Signature:       "valid-signature",
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		failures++
		if !strings.Contains(err.Error(), "pairing session already used") && !strings.Contains(err.Error(), "database table is locked") {
			t.Fatalf("expected already-used or sqlite lock error for losing concurrent claim, got %v", err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly one success and one failure, got successes=%d failures=%d", successes, failures)
	}

	session, err := pairingRepo.GetPairingSession(start.PairingSessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.UsedAt == nil || session.ClaimedByDeviceID == "" {
		t.Fatalf("expected used session with claimed device id, got %+v", session)
	}
	devices, err := svc.userRepo.GetUserDevices(user.ID)
	if err != nil {
		t.Fatalf("load user devices: %v", err)
	}
	newDevices := 0
	for _, d := range devices {
		if strings.HasPrefix(d.DeviceID, "new-device-") {
			newDevices++
		}
	}
	if newDevices != 1 {
		t.Fatalf("expected exactly one newly paired device, got %d devices=%+v", newDevices, devices)
	}
}

func newDevicePairingTestService(t *testing.T) (*DevicePairingService, *repository.DevicePairingRepo, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Device{}, &model.DevicePairingSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userRepo := repository.NewUserRepo(db)
	pairingRepo := repository.NewDevicePairingRepo(db)
	user := &model.User{PubKeyEd25519: "pubkey-1", DisplayName: "Test User", Status: "active"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	device := &model.Device{UserID: user.ID, DeviceID: "old-device-1", DeviceName: "Old Device", DevicePubKey: "old-pubkey", PairedAt: time.Now()}
	if err := userRepo.CreateDevice(device); err != nil {
		t.Fatalf("create device: %v", err)
	}
	return NewDevicePairingService(pairingRepo, userRepo, &stubVerifier{valid: true}), pairingRepo, user
}

func decodeQRPayloadForTest(t *testing.T, encoded string) DevicePairingQRPayload {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode qr payload: %v", err)
	}
	var payload DevicePairingQRPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal qr payload: %v", err)
	}
	return payload
}
