package service

import (
	"fmt"

	"github.com/agent-os/backend/internal/config"
)

type VerifierInfo interface {
	Backend() string
	Version() string
}

type ClosableVerifier interface {
	SignatureVerifier
	Close() error
}

func NewSignatureVerifierFromConfig(cfg config.IdentityVerifierConfig, appEnv string) (SignatureVerifier, func() error, error) {
	switch cfg.Backend {
	case "ffi":
		verifier, err := NewFFIVerifier(cfg.FFILibraryPath)
		if err != nil {
			return nil, nil, err
		}
		return verifier, verifier.Close, nil
	case "go":
		if appEnv == "production" {
			return nil, nil, fmt.Errorf("go identity verifier is not allowed in production")
		}
		return NewGoEd25519Verifier(), func() error { return nil }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported IDENTITY_VERIFY_BACKEND %q", cfg.Backend)
	}
}

func SignatureVerifierBackend(verifier SignatureVerifier) string {
	if info, ok := verifier.(VerifierInfo); ok {
		return info.Backend()
	}
	return "go"
}

func SignatureVerifierVersion(verifier SignatureVerifier) string {
	if info, ok := verifier.(VerifierInfo); ok {
		return info.Version()
	}
	return "builtin"
}

func (v *GoEd25519Verifier) Backend() string { return "go" }
func (v *GoEd25519Verifier) Version() string { return "builtin" }
