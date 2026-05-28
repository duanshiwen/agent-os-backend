package service

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

// SignatureVerifier validates Ed25519 signatures over server-issued challenges.
type SignatureVerifier interface {
	VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error)
}

// GoEd25519Verifier verifies signatures in-process. It is useful for tests and development fallback.
type GoEd25519Verifier struct{}

func NewGoEd25519Verifier() *GoEd25519Verifier { return &GoEd25519Verifier{} }

func (v *GoEd25519Verifier) VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error) {
	_ = ctx
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return false, fmt.Errorf("invalid public key encoding: %w", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key length: expected %d, got %d", ed25519.PublicKeySize, len(pubKeyBytes))
	}

	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature encoding: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid signature length: expected %d, got %d", ed25519.SignatureSize, len(sigBytes))
	}

	return ed25519.Verify(pubKeyBytes, []byte(challenge), sigBytes), nil
}
