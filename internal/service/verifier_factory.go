package service

import "github.com/agent-os/backend/internal/config"

type VerifierInfo interface {
	Backend() string
	Version() string
}

type ClosableVerifier interface {
	SignatureVerifier
	Close() error
}

func NewSignatureVerifierFromConfig(cfg config.IdentityVerifierConfig) (SignatureVerifier, func() error, error) {
	verifier, err := NewFFIVerifier(cfg.FFILibraryPath)
	if err != nil {
		return nil, nil, err
	}
	return verifier, verifier.Close, nil
}

func SignatureVerifierBackend(verifier SignatureVerifier) string {
	if info, ok := verifier.(VerifierInfo); ok {
		return info.Backend()
	}
	return "unknown"
}

func SignatureVerifierVersion(verifier SignatureVerifier) string {
	if info, ok := verifier.(VerifierInfo); ok {
		return info.Version()
	}
	return "unknown"
}
