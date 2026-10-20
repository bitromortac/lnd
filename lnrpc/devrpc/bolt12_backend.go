package devrpc

import (
	"context"
)

// Bolt12Backend serves the BOLT 12 methods of the Dev service that need the
// internals of the node.
type Bolt12Backend interface {
	// DecodeOffer decodes an offer string and returns its fields.
	DecodeOffer(ctx context.Context,
		req *DecodeOfferRequest) (*DecodeOfferResponse, error)

	// RequestInvoice negotiates an invoice for an offer with its issuer
	// and returns the validated invoice.
	RequestInvoice(ctx context.Context,
		req *RequestInvoiceRequest) (*RequestInvoiceResponse, error)

	// PayOffer pays an offer and streams the negotiation and payment
	// updates.
	PayOffer(req *PayOfferRequest, stream Dev_PayOfferServer) error

	// CreateInvoiceRequest publishes an invoice request without an
	// offer.
	CreateInvoiceRequest(ctx context.Context,
		req *CreateInvoiceRequestRequest) (
		*CreateInvoiceRequestResponse, error)

	// SendInvoice answers an invoice request without an offer as the
	// payee.
	SendInvoice(ctx context.Context,
		req *SendInvoiceRequest) (*SendInvoiceResponse, error)

	// ListInvoiceRequests returns the published invoice requests with the
	// invoices that wait for approval.
	ListInvoiceRequests(ctx context.Context,
		req *ListInvoiceRequestsRequest) (*ListInvoiceRequestsResponse,
		error)

	// ApproveInvoiceRequestPayment pays an invoice that waits for
	// approval.
	ApproveInvoiceRequestPayment(ctx context.Context,
		req *ApproveInvoiceRequestPaymentRequest) (
		*ApproveInvoiceRequestPaymentResponse, error)
}
