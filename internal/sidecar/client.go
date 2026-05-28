package sidecar

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/agent-os/backend/internal/sidecar/sidecarpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC connection to the Rust sidecar.
type Client struct {
	conn     *grpc.ClientConn
	socket   string
	health   sidecarpb.SidecarHealthServiceClient
	identity sidecarpb.IdentityCoreServiceClient
}

// HealthStatus is the Go-facing sidecar health summary.
type HealthStatus struct {
	Status              string   `json:"status"`
	SDKVersion          string   `json:"sdk_version"`
	EnabledCapabilities []string `json:"enabled_capabilities"`
}

// NewClient creates a new sidecar client connected via Unix socket.
func NewClient(socketPath string) (*Client, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sidecar: %w", err)
	}

	return &Client{
		conn:     conn,
		socket:   socketPath,
		health:   sidecarpb.NewSidecarHealthServiceClient(conn),
		identity: sidecarpb.NewIdentityCoreServiceClient(conn),
	}, nil
}

// HealthCheck pings the sidecar to verify it's alive.
func (c *Client) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, err := c.health.Check(ctx, &sidecarpb.HealthCheckRequest{})
	if err != nil {
		return nil, fmt.Errorf("sidecar health check failed: %w", err)
	}
	return &HealthStatus{
		Status:              resp.Status,
		SDKVersion:          resp.SdkVersion,
		EnabledCapabilities: resp.EnabledCapabilities,
	}, nil
}

// VerifyEd25519Challenge verifies an Ed25519 signature using the Rust identity-core provider.
func (c *Client) VerifyEd25519Challenge(ctx context.Context, challenge, signatureHex, publicKeyHex string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resp, err := c.identity.VerifyEd25519Challenge(ctx, &sidecarpb.VerifyEd25519ChallengeRequest{
		Challenge:    challenge,
		SignatureHex: signatureHex,
		PublicKeyHex: publicKeyHex,
	})
	if err != nil {
		return false, fmt.Errorf("sidecar identity verification call failed: %w", err)
	}
	if resp.ErrorCode != "" {
		return false, fmt.Errorf("sidecar identity verification failed: %s: %s", resp.ErrorCode, resp.ErrorMessage)
	}
	return resp.Valid, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Conn returns the underlying gRPC connection for registering service stubs.
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}
