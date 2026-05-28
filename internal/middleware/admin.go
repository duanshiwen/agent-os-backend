package middleware

import (
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// AdminMiddleware is a simple placeholder for admin access control.
// In production, this should check against an admin_users table or role field.
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For now, all authenticated users can access admin routes
		// TODO: implement proper admin role checking
		userID := MustGetUserID(c)
		if userID.String() == "" {
			response.Forbidden(c, "admin access required")
			c.Abort()
			return
		}
		c.Next()
	}
}
