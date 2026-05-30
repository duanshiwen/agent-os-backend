package handler

import (
	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func enforceSensitiveConfirmation(c *gin.Context, sensitiveOperationSvc *service.SensitiveOperationService, auditSvc *service.AuditService, confirmationToken, operation, consumedBy, deniedAuditAction, resourceType, resourceID string) bool {
	if sensitiveOperationSvc == nil {
		response.InternalError(c, "sensitive operation service unavailable")
		return false
	}
	userID := middleware.MustGetUserID(c)
	if err := sensitiveOperationSvc.ConsumeConfirmation(userID, confirmationToken, operation, consumedBy); err != nil {
		recordAuditFromContext(c, auditSvc, deniedAuditAction, resourceType, resourceID, service.AuditOutcomeDenied, gin.H{"error": err.Error(), "reason": "confirmation_required", "operation": operation})
		response.Forbidden(c, err.Error())
		return false
	}
	return true
}

func recordAuditFromContext(c *gin.Context, auditSvc *service.AuditService, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	if auditSvc == nil {
		return
	}
	actorID := middleware.MustGetUserID(c)
	_, _ = auditSvc.Record(service.RecordAuditEventInput{
		ActorUserID:   &actorID,
		ActorDeviceID: middleware.MustGetDeviceID(c),
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Outcome:       outcome,
		IPAddress:     c.ClientIP(),
		UserAgent:     c.Request.UserAgent(),
		Metadata:      metadata,
	})
}
