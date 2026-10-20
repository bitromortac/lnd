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
}
