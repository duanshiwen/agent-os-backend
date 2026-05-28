package service

import "context"

// SidecarIdentityClient is the subset of the Rust sidecar client needed for identity verification.
type SidecarIdentityClient interface {
	VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error)
}

// SidecarVerifier delegates Ed25519 verification to the Rust sidecar.
type SidecarVerifier struct {
	client SidecarIdentityClient
}

func NewSidecarVerifier(client SidecarIdentityClient) *SidecarVerifier {
	return &SidecarVerifier{client: client}
}

func (v *SidecarVerifier) VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error) {
	return v.client.VerifyEd25519Challenge(ctx, challenge, signatureHex, publicKeyHex)
}
