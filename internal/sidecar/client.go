package sidecar

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC connection to the Rust sidecar.
type Client struct {
	conn   *grpc.ClientConn
	socket string
}

// NewClient creates a new sidecar client connected via Unix socket.
func NewClient(socketPath string) (*Client, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return net.Dial("unix", addr)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sidecar: %w", err)
	}

	return &Client{
		conn:   conn,
		socket: socketPath,
	}, nil
}

// HealthCheck pings the sidecar to verify it's alive.
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Simple connectivity check — actual health check via gRPC Health service
	state := c.conn.GetState()
	log.Printf("sidecar connection state: %v", state)

	// TODO: implement proper gRPC health check when proto is defined
	_ = ctx
	return nil
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
