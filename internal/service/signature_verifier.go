package service

import "context"

// SignatureVerifier validates Ed25519 signatures over server-issued challenges.
type SignatureVerifier interface {
	VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error)
}
