package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// FFIVerifier verifies signatures through the AgentOS Rust SDK dynamic library.
type FFIVerifier struct {
	handle uintptr

	ffiVersion      func() *byte
	verifyChallenge func(challenge string, signatureHex string, publicKeyHex string, errorOut **byte) int32
	freeString      func(ptr *byte)
	libraryPath     string
	libraryVersion  string
}

func NewFFIVerifier(libraryPath string) (*FFIVerifier, error) {
	if libraryPath == "" {
		var err error
		libraryPath, err = DefaultAgentOSFFILibraryPath()
		if err != nil {
			return nil, err
		}
	}

	handle, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load AgentOS FFI library %q: %w", libraryPath, err)
	}

	v := &FFIVerifier{handle: handle, libraryPath: libraryPath}
	purego.RegisterLibFunc(&v.ffiVersion, handle, "agentos_ffi_version")
	purego.RegisterLibFunc(&v.verifyChallenge, handle, "agentos_identity_verify_ed25519_challenge")
	purego.RegisterLibFunc(&v.freeString, handle, "agentos_ffi_free_string")
	v.libraryVersion = cStringToGo(v.ffiVersion())
	return v, nil
}

func (v *FFIVerifier) VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error) {
	_ = ctx
	var errPtr *byte
	code := v.verifyChallenge(challenge, signatureHex, publicKeyHex, &errPtr)
	if errPtr != nil {
		defer v.freeString(errPtr)
	}

	switch code {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		msg := "unknown ffi error"
		if errPtr != nil {
			msg = cStringToGo(errPtr)
		}
		return false, fmt.Errorf("agentos ffi verifier error code %d: %s", code, msg)
	}
}

func (v *FFIVerifier) Close() error {
	if v == nil || v.handle == 0 {
		return nil
	}
	err := purego.Dlclose(v.handle)
	v.handle = 0
	return err
}

func (v *FFIVerifier) Backend() string     { return "ffi" }
func (v *FFIVerifier) Version() string     { return v.libraryVersion }
func (v *FFIVerifier) LibraryPath() string { return v.libraryPath }

func DefaultAgentOSFFILibraryPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve AgentOS FFI library path: runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	libName := "libagentos_ffi.dylib"
	if runtime.GOOS == "linux" {
		libName = "libagentos_ffi.so"
	}
	path := filepath.Join(root, "internal", "runtime", runtime.GOOS+"-"+runtime.GOARCH, libName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("AgentOS FFI library not found at %s: %w", path, err)
	}
	return path, nil
}

func cStringToGo(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var bytes []byte
	for p := uintptr(unsafe.Pointer(ptr)); ; p++ {
		b := *(*byte)(unsafe.Pointer(p))
		if b == 0 {
			break
		}
		bytes = append(bytes, b)
	}
	return string(bytes)
}
