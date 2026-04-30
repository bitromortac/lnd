package commands

import (
	"fmt"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/urfave/cli"
)

// CreateOfferCommand defines the lncli createoffer command.
var CreateOfferCommand = cli.Command{
	Name:     "createoffer",
	Category: "Offers",
	Usage:    "Create a new BOLT 12 offer.",
	Description: `
	Create a new BOLT 12 offer and persist it in the offer store.
	The encoded offer string (lno1...) is returned for sharing
	with payers.

	Offers without an amount can be created by omitting the
	--amt_msat flag. These offers allow the payer to specify the
	amount they wish to pay.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "description",
			Usage: "a description of the payment purpose " +
				"to include in the offer " +
				"(required when amount is set)",
		},
		cli.Uint64Flag{
			Name: "amt_msat",
			Usage: "the amount in millisatoshis per item " +
				"(0 or omit for any-amount offers)",
		},
		cli.Uint64Flag{
			Name: "absolute_expiry",
			Usage: "seconds since epoch after which the " +
				"offer expires (0 or omit for no " +
				"expiry)",
		},
		cli.Uint64Flag{
			Name: "quantity_max",
			Usage: "maximum number of items per invoice " +
				"(0 = unlimited, omit to disable " +
				"quantity selection)",
		},
	},
	Action: actionDecorator(createOffer),
}

// DecodeOfferCommand defines the lncli decodeoffer command.
var DecodeOfferCommand = cli.Command{
	Name:     "decodeoffer",
	Category: "Offers",
	Usage:    "Decode a BOLT 12 offer string.",
	Description: `
	Decode a bech32-encoded BOLT 12 offer string (lno1...) and
	display its fields. This is a stateless utility analogous to
	decodepayreq for BOLT 11.`,
	ArgsUsage: "offer_string",
	Action:    actionDecorator(decodeOffer),
}

func decodeOffer(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	if ctx.NArg() == 0 {
		return fmt.Errorf("offer_string argument required")
	}

	resp, err := client.DecodeOffer(
		ctxc, &lnrpc.DecodeOfferRequest{
			Offer: ctx.Args().First(),
		},
	)
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}

// RequestInvoiceCommand defines the lncli requestinvoice command.
var RequestInvoiceCommand = cli.Command{
	Name:     "requestinvoice",
	Category: "Offers",
	Usage: "Request a BOLT 12 invoice for an offer " +
		"from a connected peer.",
	Description: `
	Takes a BOLT 12 offer string, constructs and signs an
	invoice request, sends it to the offer's issuer via onion
	message, waits for the invoice reply, validates it, and
	returns the decoded BOLT 12 invoice. No payment is
	dispatched.`,
	ArgsUsage: "offer_string",
	Flags: []cli.Flag{
		cli.Uint64Flag{
			Name: "amt_msat",
			Usage: "the amount in millisatoshis " +
				"(required when the offer has " +
				"no fixed amount)",
		},
		cli.Uint64Flag{
			Name: "quantity",
			Usage: "the number of items to request " +
				"(required when the offer " +
				"supports quantity)",
		},
		cli.StringFlag{
			Name:  "payer_note",
			Usage: "an optional note to the payee",
		},
		cli.Uint64Flag{
			Name: "timeout",
			Usage: "seconds to wait for the invoice " +
				"reply (default 30)",
			Value: 30,
		},
	},
	Action: actionDecorator(requestInvoice),
}

func requestInvoice(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	if ctx.NArg() == 0 {
		return fmt.Errorf("offer_string argument required")
	}

	resp, err := client.RequestInvoice(
		ctxc, &lnrpc.RequestInvoiceRequest{
			Offer:          ctx.Args().First(),
			AmountMsat:     ctx.Uint64("amt_msat"),
			Quantity:       ctx.Uint64("quantity"),
			PayerNote:      ctx.String("payer_note"),
			TimeoutSeconds: ctx.Uint64("timeout"),
		},
	)
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}

func createOffer(ctx *cli.Context) error {
	ctxc := getContext()
	client, cleanUp := getClient(ctx)
	defer cleanUp()

	req := &lnrpc.CreateOfferRequest{
		Description:    ctx.String("description"),
		AmountMsat:     ctx.Uint64("amt_msat"),
		AbsoluteExpiry: ctx.Uint64("absolute_expiry"),
	}

	if ctx.IsSet("quantity_max") {
		qty := ctx.Uint64("quantity_max")
		req.QuantityMax = &qty
	}

	resp, err := client.CreateOffer(ctxc, req)
	if err != nil {
		return err
	}

	printRespJSON(resp)

	return nil
}
