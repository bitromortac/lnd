package commands

import (
	"encoding/hex"
	"fmt"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/urfave/cli"
)

// CreateInvoiceRequestCommand defines the lncli createinvoicerequest command.
var CreateInvoiceRequestCommand = cli.Command{
	Name:     "createinvoicerequest",
	Category: "Offers",
	Usage:    "Publish a BOLT 12 invoice request to send money.",
	Description: `
	Publish a BOLT 12 invoice request without an offer (lnr1...).
	The request is an offer to send money, for example a refund or a
	withdrawal. The payee answers it with an invoice, and the node
	pays that invoice at most once.

	With --expected_node_id the node pays an invoice from that node
	at once. Without it, each invoice waits for approval with
	approveinvoicerequestpayment.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "description",
			Usage: "the purpose of the payment",
		},
		cli.Uint64Flag{
			Name:  "amt_msat",
			Usage: "the amount in millisatoshis to send",
		},
		cli.Uint64Flag{
			Name: "absolute_expiry",
			Usage: "seconds since epoch after which the node no " +
				"longer pays (0 or omit for no expiry)",
		},
		cli.StringFlag{
			Name: "expected_node_id",
			Usage: "the hex node id the invoice must come " +
				"from to be paid at once",
		},
		cli.Int64Flag{
			Name: "fee_limit_msat",
			Usage: "the maximum routing fee in millisatoshis for " +
				"paying an invoice from the expected node",
		},
		cli.StringFlag{
			Name: "idempotency_key",
			Usage: "a hex key that names the request; a call " +
				"with a known key returns the stored request",
		},
	},
	Action: actionDecorator(createInvoiceRequest),
}

func createInvoiceRequest(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	req := &lnrpc.CreateInvoiceRequestRequest{
		Description:    ctx.String("description"),
		AmountMsat:     ctx.Uint64("amt_msat"),
		AbsoluteExpiry: ctx.Uint64("absolute_expiry"),
		FeeLimitMsat:   ctx.Int64("fee_limit_msat"),
	}

	var err error
	if ctx.IsSet("expected_node_id") {
		req.ExpectedNodeId, err = hex.DecodeString(
			ctx.String("expected_node_id"),
		)
		if err != nil {
			return fmt.Errorf("invalid expected_node_id: %w", err)
		}
	}

	if ctx.IsSet("idempotency_key") {
		req.IdempotencyKey, err = hex.DecodeString(
			ctx.String("idempotency_key"),
		)
		if err != nil {
			return fmt.Errorf("invalid idempotency_key: %w", err)
		}
	}

	resp, err := client.CreateInvoiceRequest(ctxc, req)
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}

// SendInvoiceCommand defines the lncli sendinvoice command.
var SendInvoiceCommand = cli.Command{
	Name:      "sendinvoice",
	Category:  "Offers",
	Usage:     "Answer a BOLT 12 invoice request to receive money.",
	ArgsUsage: "invoice_request",
	Description: `
	Answer a BOLT 12 invoice request without an offer (lnr1...) with
	an invoice for its amount. The node stores the invoice and sends
	it to the payer in an onion message.`,
	Action: actionDecorator(sendInvoice),
}

func sendInvoice(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	if ctx.NArg() != 1 {
		return cli.ShowCommandHelp(ctx, "sendinvoice")
	}

	resp, err := client.SendInvoice(ctxc, &lnrpc.SendInvoiceRequest{
		InvoiceRequest: ctx.Args().First(),
	})
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}

// ListInvoiceRequestsCommand defines the lncli listinvoicerequests command.
var ListInvoiceRequestsCommand = cli.Command{
	Name:     "listinvoicerequests",
	Category: "Offers",
	Usage:    "List the published BOLT 12 invoice requests.",
	Description: `
	List the invoice requests without an offer that this node
	published, with the invoices that wait for approval.`,
	Action: actionDecorator(listInvoiceRequests),
}

func listInvoiceRequests(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	resp, err := client.ListInvoiceRequests(
		ctxc, &lnrpc.ListInvoiceRequestsRequest{},
	)
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}

// ApproveInvoiceRequestPaymentCommand defines the lncli
// approveinvoicerequestpayment command.
var ApproveInvoiceRequestPaymentCommand = cli.Command{
	Name:      "approveinvoicerequestpayment",
	Category:  "Offers",
	Usage:     "Pay an invoice that waits for approval.",
	ArgsUsage: "payment_hash",
	Description: `
	Pay an invoice for a published invoice request that waits for
	approval, because the request names no expected node. Check the
	invoice_node_id in listinvoicerequests first.`,
	Flags: []cli.Flag{
		cli.Int64Flag{
			Name:  "fee_limit_msat",
			Usage: "the maximum routing fee in millisatoshis",
		},
	},
	Action: actionDecorator(approveInvoiceRequestPayment),
}

func approveInvoiceRequestPayment(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	if ctx.NArg() != 1 {
		return cli.ShowCommandHelp(ctx, "approveinvoicerequestpayment")
	}

	hash, err := hex.DecodeString(ctx.Args().First())
	if err != nil {
		return fmt.Errorf("invalid payment_hash: %w", err)
	}

	resp, err := client.ApproveInvoiceRequestPayment(
		ctxc, &lnrpc.ApproveInvoiceRequestPaymentRequest{
			PaymentHash:  hash,
			FeeLimitMsat: ctx.Int64("fee_limit_msat"),
		},
	)
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}
