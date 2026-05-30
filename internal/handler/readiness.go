package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReadinessHandler struct {
	db *gorm.DB
}

func NewReadinessHandler(db *gorm.DB) *ReadinessHandler {
	return &ReadinessHandler{db: db}
}

func (h *ReadinessHandler) Ready(c *gin.Context) {
	dbOK := false
	if h.db != nil {
		if sqlDB, err := h.db.DB(); err == nil && sqlDB.PingContext(c.Request.Context()) == nil {
			dbOK = true
		}
	}
	statusCode := http.StatusOK
	status := "ready"
	if !dbOK {
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
	}
	c.JSON(statusCode, gin.H{"status": status, "checks": gin.H{"database": dbOK}})
}
