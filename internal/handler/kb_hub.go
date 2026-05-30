package handler

import (
	"errors"
	"net/http"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type KBHubHandler struct {
	svc *service.KBHubService
}

func NewKBHubHandler(svc *service.KBHubService) *KBHubHandler {
	return &KBHubHandler{svc: svc}
}

func (h *KBHubHandler) CreateCollection(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	var req service.CreateKBCollectionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	collection, err := h.svc.CreateCollection(userID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, collection)
}

func (h *KBHubHandler) ListCollections(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collections, err := h.svc.ListCollections(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, collections)
}

func (h *KBHubHandler) GetCollection(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	collection, err := h.svc.GetCollection(userID, collectionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, collection)
}

func (h *KBHubHandler) PublishSnapshot(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.PublishKBSnapshotInput
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	snapshot, err := h.svc.PublishSnapshot(c.Request.Context(), userID, collectionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, snapshot)
}

func (h *KBHubHandler) ListSnapshots(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshots, err := h.svc.ListSnapshots(userID, collectionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, snapshots)
}

func (h *KBHubHandler) GetSnapshot(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	snapshot, err := h.svc.GetSnapshot(userID, collectionID, snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, snapshot)
}

func (h *KBHubHandler) ListPublicCollections(c *gin.Context) {
	collections, err := h.svc.ListPublicCollections()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, collections)
}

func (h *KBHubHandler) GetPublicCollection(c *gin.Context) {
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	collection, err := h.svc.GetPublicCollection(collectionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, collection)
}

func (h *KBHubHandler) GetPublicSnapshot(c *gin.Context) {
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	snapshot, err := h.svc.GetPublicSnapshot(collectionID, snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, snapshot)
}

func (h *KBHubHandler) CreateManifestDownloadURL(c *gin.Context) {
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	result, err := h.svc.CreateSnapshotManifestDownloadURL(c.Request.Context(), collectionID, snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *KBHubHandler) InstallCollection(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.InstallKBCollectionInput
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	subscription, err := h.svc.InstallCollection(userID, collectionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Created(c, subscription)
}

func (h *KBHubHandler) ListSubscriptions(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	subscriptions, err := h.svc.ListSubscriptions(userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, subscriptions)
}

func (h *KBHubHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrKBCollectionNotFound), errors.Is(err, service.ErrKBSnapshotNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrKBNoEntries):
		response.Conflict(c, err.Error())
	case errors.Is(err, service.ErrKBInvalid):
		response.BadRequest(c, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, err.Error())
	}
}
