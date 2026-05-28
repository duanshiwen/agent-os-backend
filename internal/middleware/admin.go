package middleware

import (
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

// AdminMiddleware allows only authenticated users with an explicit admin marker.
// A user is considered admin when either is_admin=true or role is "admin" / "owner".
func AdminMiddleware(userRepo *repository.UserRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			response.Forbidden(c, "admin access required")
			c.Abort()
			return
		}

		userID := MustGetUserID(c)
		if userID.String() == "" {
			response.Forbidden(c, "admin access required")
			c.Abort()
			return
		}

		user, err := userRepo.GetByID(userID)
		if err != nil || user == nil || user.Status != "active" || (!user.IsAdmin && user.Role != "admin" && user.Role != "owner") {
			response.Forbidden(c, "admin access required")
			c.Abort()
			return
		}

		c.Next()
	}
}
