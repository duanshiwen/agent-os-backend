package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/agent-os/backend/internal/middleware"
	"github.com/agent-os/backend/internal/pkg/response"
	"github.com/agent-os/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type KBHubHandler struct {
	svc        *service.KBHubService
	searchSvc  *service.KBSearchService
	billingSvc *service.KBBillingService
	auditSvc   *service.AuditService
}

func NewKBHubHandler(svc *service.KBHubService) *KBHubHandler {
	return &KBHubHandler{svc: svc}
}

func (h *KBHubHandler) SetSearchService(searchSvc *service.KBSearchService) {
	h.searchSvc = searchSvc
}

func (h *KBHubHandler) SetBillingService(billingSvc *service.KBBillingService) {
	h.billingSvc = billingSvc
}

func (h *KBHubHandler) SetAuditService(auditSvc *service.AuditService) {
	h.auditSvc = auditSvc
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

func (h *KBHubHandler) ArchiveSnapshot(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	snapshot, err := h.svc.ArchiveSnapshot(userID, collectionID, snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBSnapshotArchived, "kb_snapshot", snapshotID.String(), service.AuditOutcomeSuccess, gin.H{"collection_id": collectionID.String()})
	response.OK(c, snapshot)
}

func (h *KBHubHandler) RestoreSnapshot(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	snapshot, err := h.svc.RestoreSnapshot(userID, collectionID, snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBSnapshotRestored, "kb_snapshot", snapshotID.String(), service.AuditOutcomeSuccess, gin.H{"collection_id": collectionID.String()})
	response.OK(c, snapshot)
}

func (h *KBHubHandler) DiffSnapshots(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	fromID, err := uuid.Parse(c.Query("from_snapshot_id"))
	if err != nil {
		response.BadRequest(c, "invalid from_snapshot_id")
		return
	}
	toID, err := uuid.Parse(c.Query("to_snapshot_id"))
	if err != nil {
		response.BadRequest(c, "invalid to_snapshot_id")
		return
	}
	diff, err := h.svc.DiffSnapshots(userID, collectionID, fromID, toID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, diff)
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

func (h *KBHubHandler) RetrySnapshotEmbeddingJobs(c *gin.Context) {
	if h.searchSvc == nil {
		response.InternalError(c, "kb search service not configured")
		return
	}
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	if _, err := h.svc.GetSnapshot(userID, collectionID, snapshotID); err != nil {
		h.handleError(c, err)
		return
	}
	result, err := h.searchSvc.RetryFailedEmbeddingJobs(snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *KBHubHandler) GetSnapshotEmbeddingStatus(c *gin.Context) {
	if h.searchSvc == nil {
		response.InternalError(c, "kb search service not configured")
		return
	}
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return
	}
	if _, err := h.svc.GetSnapshot(userID, collectionID, snapshotID); err != nil {
		h.handleError(c, err)
		return
	}
	status, err := h.searchSvc.EmbeddingStatus(c.Request.Context(), snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, status)
}

func (h *KBHubHandler) ListPublicCollections(c *gin.Context) {
	input, ok := parseSearchCollectionsQuery(c)
	if !ok {
		return
	}
	result, err := h.svc.SearchPublicCollections(input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, result)
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

func (h *KBHubHandler) UpdateCollectionDeclarations(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateKBCollectionDeclarationsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	collection, err := h.svc.UpdateCollectionDeclarations(userID, collectionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, collection)
}

func (h *KBHubHandler) ReportCollection(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.ReportKBCollectionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	report, err := h.svc.ReportCollection(userID, collectionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBCollectionReported, "kb_collection", collectionID.String(), service.AuditOutcomeSuccess, gin.H{"reason": report.Reason, "report_id": report.ID.String()})
	response.Created(c, report)
}

func (h *KBHubHandler) UpdateCollectionPricing(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateKBCollectionPricingInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	collection, err := h.svc.UpdateCollectionPricing(userID, collectionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, collection)
}

func (h *KBHubHandler) ListCollectionBillingPlans(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.GetCollection(userID, collectionID); err != nil {
		h.handleError(c, err)
		return
	}
	plans, err := h.billingSvc.ListBillingPlans(collectionID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, plans)
}

func (h *KBHubHandler) GetCollectionStats(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	stats, err := h.svc.GetCollectionStats(userID, collectionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, stats)
}

func (h *KBHubHandler) Search(c *gin.Context) {
	if h.searchSvc == nil {
		response.InternalError(c, "kb search service not configured")
		return
	}
	input, ok := parseKBSearchQuery(c)
	if !ok {
		return
	}
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uuid.UUID); ok {
			input.UserID = id
		}
	}
	result, err := h.searchSvc.Search(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrKBSemanticSearchUnavailable) {
			response.Error(c, http.StatusNotImplemented, err.Error())
			return
		}
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

func (h *KBHubHandler) CancelSubscription(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	subscription, err := h.svc.CancelSubscription(userID, collectionID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, subscription)
}

func (h *KBHubHandler) CreateInstalledManifestDownloadURL(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, snapshotID, ok := parseAccessSnapshotParams(c)
	if !ok {
		return
	}
	result, err := h.svc.CreateInstalledManifestDownloadURL(c.Request.Context(), userID, collectionID, snapshotID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *KBHubHandler) CreateInstalledEntryContentDownloadURL(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, snapshotID, ok := parseAccessSnapshotParams(c)
	if !ok {
		return
	}
	entryRecordID, ok := parseUUIDParam(c, "entry_id")
	if !ok {
		return
	}
	result, err := h.svc.CreateInstalledEntryContentDownloadURL(c.Request.Context(), userID, collectionID, snapshotID, entryRecordID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *KBHubHandler) FetchInstalledEntryFullText(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	collectionID, snapshotID, ok := parseAccessSnapshotParams(c)
	if !ok {
		return
	}
	entryRecordID, ok := parseUUIDParam(c, "entry_id")
	if !ok {
		return
	}
	result, err := h.svc.FetchInstalledEntryFullText(c.Request.Context(), userID, collectionID, snapshotID, entryRecordID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, result)
}

func (h *KBHubHandler) GetBillingAccount(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	account, err := h.billingSvc.GetAccount(middleware.MustGetUserID(c))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, account)
}

func (h *KBHubHandler) ListBillingTransactions(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	txns, err := h.billingSvc.ListTransactions(middleware.MustGetUserID(c), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, txns)
}

func (h *KBHubHandler) ListBillingInvoices(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	invoices, err := h.billingSvc.ListInvoices(middleware.MustGetUserID(c), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, invoices)
}

func (h *KBHubHandler) GetBillingInvoice(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	invoiceID, ok := parseUUIDParam(c, "invoice_id")
	if !ok {
		return
	}
	detail, err := h.billingSvc.GetInvoiceDetail(middleware.MustGetUserID(c), invoiceID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, detail)
}

func (h *KBHubHandler) RequestBillingRefund(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	var req service.RequestKBRefundInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	refund, err := h.billingSvc.RequestRefund(middleware.MustGetUserID(c), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBRefundRequested, "kb_refund", refund.ID.String(), service.AuditOutcomeSuccess, gin.H{"invoice_id": refund.InvoiceID.String(), "amount": refund.Amount})
	response.Created(c, refund)
}

func (h *KBHubHandler) ListBillingRefunds(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	refunds, err := h.billingSvc.ListRefunds(middleware.MustGetUserID(c), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, refunds)
}

func (h *KBHubHandler) OpenBillingDispute(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	var req service.OpenKBBillingDisputeInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	dispute, err := h.billingSvc.OpenDispute(middleware.MustGetUserID(c), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBBillingDisputeOpened, "kb_billing_dispute", dispute.ID.String(), service.AuditOutcomeSuccess, gin.H{"invoice_id": dispute.InvoiceID.String(), "reason": dispute.Reason})
	response.Created(c, dispute)
}

func (h *KBHubHandler) ListBillingDisputes(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	disputes, err := h.billingSvc.ListDisputes(middleware.MustGetUserID(c), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, disputes)
}

func (h *KBHubHandler) AdminCreateBillingInvoice(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	var req service.CreateKBInvoiceInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	detail, err := h.billingSvc.CreateInvoice(req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBInvoiceIssued, "kb_invoice", detail.Invoice.ID.String(), service.AuditOutcomeSuccess, gin.H{"user_id": detail.Invoice.UserID.String(), "total": detail.Invoice.Total})
	response.Created(c, detail)
}

func (h *KBHubHandler) AdminMarkInvoicePaid(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	invoiceID, ok := parseUUIDParam(c, "invoice_id")
	if !ok {
		return
	}
	invoice, err := h.billingSvc.MarkInvoicePaid(invoiceID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBInvoicePaid, "kb_invoice", invoice.ID.String(), service.AuditOutcomeSuccess, gin.H{"total": invoice.Total})
	response.OK(c, invoice)
}

func (h *KBHubHandler) AdminResolveRefund(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	adminID := middleware.MustGetUserID(c)
	refundID, ok := parseUUIDParam(c, "refund_id")
	if !ok {
		return
	}
	var req service.ResolveKBRefundInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	refund, err := h.billingSvc.ResolveRefund(adminID, refundID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBRefundResolved, "kb_refund", refund.ID.String(), service.AuditOutcomeSuccess, gin.H{"status": refund.Status})
	response.OK(c, refund)
}

func (h *KBHubHandler) AdminResolveDispute(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	adminID := middleware.MustGetUserID(c)
	disputeID, ok := parseUUIDParam(c, "dispute_id")
	if !ok {
		return
	}
	var req service.ResolveKBBillingDisputeInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	dispute, err := h.billingSvc.ResolveDispute(adminID, disputeID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBBillingDisputeResolved, "kb_billing_dispute", dispute.ID.String(), service.AuditOutcomeSuccess, gin.H{"status": dispute.Status})
	response.OK(c, dispute)
}

func (h *KBHubHandler) AdminMarkPayoutPaid(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	payoutID, ok := parseUUIDParam(c, "payout_id")
	if !ok {
		return
	}
	payout, err := h.billingSvc.MarkPayoutPaid(payoutID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBPayoutPaid, "contributor_payout_period", payout.ID.String(), service.AuditOutcomeSuccess, gin.H{"period": payout.Period, "net_amount": payout.NetAmount})
	response.OK(c, payout)
}

func (h *KBHubHandler) AdminListCollectionsForReview(c *gin.Context) {
	collections, err := h.svc.ListCollectionsForReview(c.Query("review_status"), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, collections)
}

func (h *KBHubHandler) AdminReviewCollection(c *gin.Context) {
	adminID := middleware.MustGetUserID(c)
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req service.ReviewKBCollectionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	collection, err := h.svc.ReviewCollection(adminID, collectionID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBCollectionReviewed, "kb_collection", collectionID.String(), service.AuditOutcomeSuccess, gin.H{"review_status": collection.ReviewStatus, "reason": req.Reason})
	response.OK(c, collection)
}

func (h *KBHubHandler) AdminListModerationReports(c *gin.Context) {
	reports, err := h.svc.ListModerationReports(c.Query("status"), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, reports)
}

func (h *KBHubHandler) AdminResolveModerationReport(c *gin.Context) {
	adminID := middleware.MustGetUserID(c)
	reportID, ok := parseUUIDParam(c, "report_id")
	if !ok {
		return
	}
	var req service.ResolveKBModerationReportInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	report, err := h.svc.ResolveModerationReport(adminID, reportID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.OK(c, report)
}

func (h *KBHubHandler) AdminExpireSubscriptions(c *gin.Context) {
	count, err := h.svc.ExpireSubscriptions(time.Now().UTC())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	recordAuditFromContext(c, h.auditSvc, service.AuditActionKBSubscriptionsExpired, "kb_subscription", "expired", service.AuditOutcomeSuccess, gin.H{"expired": count})
	response.OK(c, gin.H{"expired": count})
}

func (h *KBHubHandler) ListContributorPayoutPeriods(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	payouts, err := h.billingSvc.ListPayoutPeriods(middleware.MustGetUserID(c), parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, payouts)
}

func (h *KBHubHandler) ListContributorEarnings(c *gin.Context) {
	if h.billingSvc == nil {
		response.InternalError(c, "billing service not configured")
		return
	}
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	earnings, err := h.billingSvc.ListContributorEarnings(middleware.MustGetUserID(c), &collectionID, parseIntQuery(c, "limit", 50), parseIntQuery(c, "offset", 0))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, earnings)
}

func parseAccessSnapshotParams(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	collectionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	snapshotID, ok := parseUUIDParam(c, "snapshot_id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return collectionID, snapshotID, true
}

func parseSearchCollectionsQuery(c *gin.Context) (service.SearchKBCollectionsInput, bool) {
	input := service.SearchKBCollectionsInput{Q: c.Query("q"), Limit: parseIntQuery(c, "limit", 20), Offset: parseIntQuery(c, "offset", 0)}
	if owner := c.Query("owner_id"); owner != "" {
		id, err := uuid.Parse(owner)
		if err != nil {
			response.BadRequest(c, "invalid owner_id")
			return input, false
		}
		input.OwnerID = &id
	}
	if raw := c.Query("is_free"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "invalid is_free")
			return input, false
		}
		input.IsFree = &value
	}
	return input, true
}

func parseKBSearchQuery(c *gin.Context) (service.KBSearchInput, bool) {
	input := service.KBSearchInput{Q: c.Query("q"), Mode: c.DefaultQuery("mode", "lexical"), Limit: parseIntQuery(c, "limit", 20), Offset: parseIntQuery(c, "offset", 0)}
	if raw := c.Query("collection_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(c, "invalid collection_id")
			return input, false
		}
		input.CollectionID = &id
	}
	if raw := c.Query("snapshot_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(c, "invalid snapshot_id")
			return input, false
		}
		input.SnapshotID = &id
	}
	return input, true
}

func parseIntQuery(c *gin.Context, key string, fallback int) int {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func (h *KBHubHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrKBCollectionNotFound), errors.Is(err, service.ErrKBSnapshotNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrKBSubscriptionNeeded):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrKBNoEntries):
		response.Conflict(c, err.Error())
	case errors.Is(err, service.ErrKBInvalid):
		response.BadRequest(c, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, err.Error())
	}
}
