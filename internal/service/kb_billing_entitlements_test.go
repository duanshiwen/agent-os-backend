package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/google/uuid"
)

func TestKBBillingEntitlementPlanAndSubscriptionPeriods(t *testing.T) {
	svc, knowledgeRepo, _, ownerID, _ := newKBHubServiceTestEnv(t)
	consumerID := uuid.New()
	if err := knowledgeRepo.Create(&model.UserKnowledgeEntry{UserID: ownerID, EntryID: "billing/plan", Title: "Plan", ContentMarkdown: "# Plan", Status: repository.KnowledgeEntryStatusActive, Version: 1, ContentHash: strings.Repeat("4", 64)}); err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	collection, err := svc.CreateCollection(ownerID, CreateKBCollectionInput{Name: "Paid KB"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	collection, err = svc.UpdateCollectionPricing(ownerID, collection.ID, UpdateKBCollectionPricingInput{IsFree: boolPtr(false), EntitlementMode: KBEntitlementPaid, BillingInterval: KBBillingIntervalMonth, MonthlyPrice: 9900, Currency: "CNY", TrialDays: 7})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if collection.EntitlementMode != KBEntitlementPaid || collection.MonthlyPrice != 9900 || collection.BillingInterval != KBBillingIntervalMonth {
		t.Fatalf("unexpected collection pricing: %+v", collection)
	}
	plans, err := svc.billingSvc.ListBillingPlans(collection.ID)
	if err != nil || len(plans) != 1 {
		t.Fatalf("expected one billing plan, plans=%+v err=%v", plans, err)
	}
	if plans[0].EntitlementType != KBEntitlementPaid || plans[0].Price != 9900 || plans[0].Status != "active" {
		t.Fatalf("unexpected plan: %+v", plans[0])
	}
	if _, err := svc.PublishSnapshot(context.Background(), ownerID, collection.ID, PublishKBSnapshotInput{}); err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	sub, err := svc.InstallCollection(consumerID, collection.ID, InstallKBCollectionInput{EntitlementType: KBEntitlementPaid})
	if err != nil {
		t.Fatalf("install paid collection: %v", err)
	}
	if sub.EntitlementType != KBEntitlementPaid || sub.RenewalStatus != "active" || sub.CurrentPeriodEnd == nil || sub.ExpiresAt == nil {
		t.Fatalf("unexpected paid subscription: %+v", sub)
	}
	if sub.CurrentPeriodEnd.Sub(*sub.CurrentPeriodStart) < 27*24*time.Hour {
		t.Fatalf("expected monthly entitlement period, got %+v", sub)
	}
	grantUserID := uuid.New()
	if _, err := svc.InstallCollection(grantUserID, collection.ID, InstallKBCollectionInput{EntitlementType: KBEntitlementGranted}); err == nil || !strings.Contains(err.Error(), "grant_reason") {
		t.Fatalf("expected grant reason validation, got %v", err)
	}
	grant, err := svc.InstallCollection(grantUserID, collection.ID, InstallKBCollectionInput{EntitlementType: KBEntitlementGranted, GrantReason: "community grant"})
	if err != nil {
		t.Fatalf("install grant: %v", err)
	}
	if grant.EntitlementType != KBEntitlementGranted || grant.GrantReason != "community grant" || grant.RenewalStatus != "none" {
		t.Fatalf("unexpected grant subscription: %+v", grant)
	}
}

func TestKBBillingInvoiceRefundDisputeAndPayout(t *testing.T) {
	svc, _, _, ownerID, _ := newKBHubServiceTestEnv(t)
	userID := uuid.New()
	collectionID := uuid.New()
	snapshotID := uuid.New()
	_, txn, earning, err := svc.billingSvc.RecordUsage(RecordKBUsageInput{UserID: userID, OwnerID: ownerID, CollectionID: collectionID, SnapshotID: snapshotID, OperationType: KBOperationEntryFullTextFetch, TokensUsed: 10, UnitPrice: 5, Currency: "CNY"})
	if err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if txn == nil || txn.Amount != -50 || earning == nil || earning.NetAmount != 45 {
		t.Fatalf("unexpected usage billing txn=%+v earning=%+v", txn, earning)
	}
	payouts, err := svc.billingSvc.ListPayoutPeriods(ownerID, 10, 0)
	if err != nil || len(payouts) != 1 {
		t.Fatalf("expected payout period, payouts=%+v err=%v", payouts, err)
	}
	invoiceDetail, err := svc.billingSvc.CreateInvoice(CreateKBInvoiceInput{UserID: userID, CollectionID: &collectionID, Currency: "CNY", Items: []CreateKBInvoiceItemInput{{Description: "Monthly KB access", Quantity: 1, UnitPrice: 9900}}})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if invoiceDetail.Invoice.Status != "issued" || invoiceDetail.Invoice.Total != 9900 || len(invoiceDetail.Items) != 1 {
		t.Fatalf("unexpected invoice detail: %+v", invoiceDetail)
	}
	paid, err := svc.billingSvc.MarkInvoicePaid(invoiceDetail.Invoice.ID)
	if err != nil {
		t.Fatalf("mark invoice paid: %v", err)
	}
	if paid.Status != "paid" || paid.PaidAt == nil {
		t.Fatalf("unexpected paid invoice: %+v", paid)
	}
	refund, err := svc.billingSvc.RequestRefund(userID, RequestKBRefundInput{InvoiceID: invoiceDetail.Invoice.ID, Amount: 1000, Reason: "duplicate purchase"})
	if err != nil {
		t.Fatalf("request refund: %v", err)
	}
	resolvedRefund, err := svc.billingSvc.ResolveRefund(uuid.New(), refund.ID, ResolveKBRefundInput{Status: "approved"})
	if err != nil {
		t.Fatalf("resolve refund: %v", err)
	}
	if resolvedRefund.Status != "approved" || resolvedRefund.ProcessedAt == nil {
		t.Fatalf("unexpected refund: %+v", resolvedRefund)
	}
	dispute, err := svc.billingSvc.OpenDispute(userID, OpenKBBillingDisputeInput{InvoiceID: invoiceDetail.Invoice.ID, Reason: "wrong amount", Detail: "expected discount"})
	if err != nil {
		t.Fatalf("open dispute: %v", err)
	}
	resolvedDispute, err := svc.billingSvc.ResolveDispute(uuid.New(), dispute.ID, ResolveKBBillingDisputeInput{Status: "accepted", Resolution: "credit issued"})
	if err != nil {
		t.Fatalf("resolve dispute: %v", err)
	}
	if resolvedDispute.Status != "accepted" || resolvedDispute.ResolvedAt == nil {
		t.Fatalf("unexpected dispute: %+v", resolvedDispute)
	}
	paidPayout, err := svc.billingSvc.MarkPayoutPaid(payouts[0].ID)
	if err != nil {
		t.Fatalf("mark payout paid: %v", err)
	}
	if paidPayout.Status != "paid" || paidPayout.PaidAt == nil {
		t.Fatalf("unexpected payout: %+v", paidPayout)
	}
}

func boolPtr(v bool) *bool { return &v }
