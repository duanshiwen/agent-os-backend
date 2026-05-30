package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
)

const RequestIDContextKey = "request_id"
const RequestIDHeader = "X-Request-ID"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set(RequestIDContextKey, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "req-unavailable"
	}
	return "req_" + hex.EncodeToString(buf)
}
