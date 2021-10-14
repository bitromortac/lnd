package channeldb

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/feature"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

var (
	emptyFeatures = lnwire.NewFeatureVector(nil, lnwire.Features)
	ampFeatures   = lnwire.NewFeatureVector(
		lnwire.NewRawFeatureVector(
			lnwire.TLVOnionPayloadOptional,
			lnwire.PaymentAddrOptional,
			lnwire.AMPRequired,
		),
		lnwire.Features,
	)
	testNow = time.Unix(1, 0)
)

func randInvoice(value lnwire.MilliSatoshi) (*Invoice, error) {
	var (
		pre     lntypes.Preimage
		payAddr [32]byte
	)
	if _, err := rand.Read(pre[:]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(payAddr[:]); err != nil {
		return nil, err
	}

	i := &Invoice{
		CreationDate: testNow,
		Terms: ContractTerm{
			Expiry:          4000,
			PaymentPreimage: &pre,
			PaymentAddr:     payAddr,
			Value:           value,
			Features:        emptyFeatures,
		},
		Htlcs:    map[CircuitKey]*InvoiceHTLC{},
		AMPState: map[SetID]InvoiceStateAMP{},
	}
	i.Memo = []byte("memo")

	// Create a random byte slice of MaxPaymentRequestSize bytes to be used
	// as a dummy paymentrequest, and  determine if it should be set based
	// on one of the random bytes.
	var r [MaxPaymentRequestSize]byte
	if _, err := rand.Read(r[:]); err != nil {
		return nil, err
	}
	if r[0]&1 == 0 {
		i.PaymentRequest = r[:]
	} else {
		i.PaymentRequest = []byte("")
	}

	return i, nil
}

// settleTestInvoice settles a test invoice.
func settleTestInvoice(invoice *Invoice, settleIndex uint64) {
	invoice.SettleDate = testNow
	invoice.AmtPaid = invoice.Terms.Value
	invoice.State = ContractSettled
	invoice.Htlcs[CircuitKey{}] = &InvoiceHTLC{
		Amt:           invoice.Terms.Value,
		AcceptTime:    testNow,
		ResolveTime:   testNow,
		State:         HtlcStateSettled,
		CustomRecords: make(record.CustomSet),
	}
	invoice.SettleIndex = settleIndex
}

// Tests that pending invoices are those which are either in ContractOpen or
// in ContractAccepted state.
func TestInvoiceIsPending(t *testing.T) {
	contractStates := []ContractState{
		ContractOpen, ContractSettled, ContractCanceled, ContractAccepted,
	}

	for _, state := range contractStates {
		invoice := Invoice{
			State: state,
		}

		// We expect that an invoice is pending if it's either in ContractOpen
		// or ContractAccepted state.
		pending := (state == ContractOpen || state == ContractAccepted)

		if invoice.IsPending() != pending {
			t.Fatalf("expected pending: %v, got: %v, invoice: %v",
				pending, invoice.IsPending(), invoice)
		}
	}
}

type invWorkflowTest struct {
	name         string
	queryPayHash bool
	queryPayAddr bool
}

var invWorkflowTests = []invWorkflowTest{
	{
		name:         "unknown",
		queryPayHash: false,
		queryPayAddr: false,
	},
	{
		name:         "only payhash known",
		queryPayHash: true,
		queryPayAddr: false,
	},
	{
		name:         "payaddr and payhash known",
		queryPayHash: true,
		queryPayAddr: true,
	},
}

// TestInvoiceWorkflow asserts the basic process of inserting, fetching, and
// updating an invoice. We assert that the flow is successful using when
// querying with various combinations of payment hash and payment address.
func TestInvoiceWorkflow(t *testing.T) {
	t.Parallel()

	for _, test := range invWorkflowTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			testInvoiceWorkflow(t, test)
		})
	}
}

func testInvoiceWorkflow(t *testing.T, test invWorkflowTest) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	// Create a fake invoice which we'll use several times in the tests
	// below.
	fakeInvoice, err := randInvoice(10000)
	if err != nil {
		t.Fatalf("unable to create invoice: %v", err)
	}
	invPayHash := fakeInvoice.Terms.PaymentPreimage.Hash()

	// Select the payment hash and payment address we will use to lookup or
	// update the invoice for the remainder of the test.
	var (
		payHash lntypes.Hash
		payAddr *[32]byte
		ref     InvoiceRef
	)
	switch {
	case test.queryPayHash && test.queryPayAddr:
		payHash = invPayHash
		payAddr = &fakeInvoice.Terms.PaymentAddr
		ref = InvoiceRefByHashAndAddr(payHash, *payAddr)
	case test.queryPayHash:
		payHash = invPayHash
		ref = InvoiceRefByHash(payHash)
	}

	// Add the invoice to the database, this should succeed as there aren't
	// any existing invoices within the database with the same payment
	// hash.
	if _, err := db.AddInvoice(fakeInvoice, invPayHash); err != nil {
		t.Fatalf("unable to find invoice: %v", err)
	}

	// Attempt to retrieve the invoice which was just added to the
	// database. It should be found, and the invoice returned should be
	// identical to the one created above.
	dbInvoice, err := db.LookupInvoice(ref)
	if !test.queryPayAddr && !test.queryPayHash {
		if err != ErrInvoiceNotFound {
			t.Fatalf("invoice should not exist: %v", err)
		}
		return
	}

	require.Equal(t,
		*fakeInvoice, dbInvoice,
		"invoice fetched from db doesn't match original",
	)

	// The add index of the invoice retrieved from the database should now
	// be fully populated. As this is the first index written to the DB,
	// the addIndex should be 1.
	if dbInvoice.AddIndex != 1 {
		t.Fatalf("wrong add index: expected %v, got %v", 1,
			dbInvoice.AddIndex)
	}

	// Settle the invoice, the version retrieved from the database should
	// now have the settled bit toggle to true and a non-default
	// SettledDate
	payAmt := fakeInvoice.Terms.Value * 2
	_, err = db.UpdateInvoice(ref, getUpdateInvoice(payAmt))
	if err != nil {
		t.Fatalf("unable to settle invoice: %v", err)
	}
	dbInvoice2, err := db.LookupInvoice(ref)
	if err != nil {
		t.Fatalf("unable to fetch invoice: %v", err)
	}
	if dbInvoice2.State != ContractSettled {
		t.Fatalf("invoice should now be settled but isn't")
	}
	if dbInvoice2.SettleDate.IsZero() {
		t.Fatalf("invoice should have non-zero SettledDate but isn't")
	}

	// Our 2x payment should be reflected, and also the settle index of 1
	// should also have been committed for this index.
	if dbInvoice2.AmtPaid != payAmt {
		t.Fatalf("wrong amt paid: expected %v, got %v", payAmt,
			dbInvoice2.AmtPaid)
	}
	if dbInvoice2.SettleIndex != 1 {
		t.Fatalf("wrong settle index: expected %v, got %v", 1,
			dbInvoice2.SettleIndex)
	}

	// Attempt to insert generated above again, this should fail as
	// duplicates are rejected by the processing logic.
	if _, err := db.AddInvoice(fakeInvoice, payHash); err != ErrDuplicateInvoice {
		t.Fatalf("invoice insertion should fail due to duplication, "+
			"instead %v", err)
	}

	// Attempt to look up a non-existent invoice, this should also fail but
	// with a "not found" error.
	var fakeHash [32]byte
	fakeRef := InvoiceRefByHash(fakeHash)
	_, err = db.LookupInvoice(fakeRef)
	if err != ErrInvoiceNotFound {
		t.Fatalf("lookup should have failed, instead %v", err)
	}

	// Add 10 random invoices.
	const numInvoices = 10
	amt := lnwire.NewMSatFromSatoshis(1000)
	invoices := make([]*Invoice, numInvoices+1)
	invoices[0] = &dbInvoice2
	for i := 1; i < len(invoices); i++ {
		invoice, err := randInvoice(amt)
		if err != nil {
			t.Fatalf("unable to create invoice: %v", err)
		}

		hash := invoice.Terms.PaymentPreimage.Hash()
		if _, err := db.AddInvoice(invoice, hash); err != nil {
			t.Fatalf("unable to add invoice %v", err)
		}

		invoices[i] = invoice
	}

	// Perform a scan to collect all the active invoices.
	query := InvoiceQuery{
		IndexOffset:    0,
		NumMaxInvoices: math.MaxUint64,
		PendingOnly:    false,
	}

	response, err := db.QueryInvoices(query)
	if err != nil {
		t.Fatalf("invoice query failed: %v", err)
	}

	// The retrieve list of invoices should be identical as since we're
	// using big endian, the invoices should be retrieved in ascending
	// order (and the primary key should be incremented with each
	// insertion).
	for i := 0; i < len(invoices); i++ {
		require.Equal(t,
			*invoices[i], response.Invoices[i],
			"retrieved invoice doesn't match",
		)
	}
}

// TestAddDuplicatePayAddr asserts that the payment addresses of inserted
// invoices are unique.
func TestAddDuplicatePayAddr(t *testing.T) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	require.NoError(t, err)

	// Create two invoices with the same payment addr.
	invoice1, err := randInvoice(1000)
	require.NoError(t, err)

	invoice2, err := randInvoice(20000)
	require.NoError(t, err)
	invoice2.Terms.PaymentAddr = invoice1.Terms.PaymentAddr

	// First insert should succeed.
	inv1Hash := invoice1.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice1, inv1Hash)
	require.NoError(t, err)

	// Second insert should fail with duplicate payment addr.
	inv2Hash := invoice2.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice2, inv2Hash)
	require.Error(t, err, ErrDuplicatePayAddr)
}

// TestAddDuplicateKeysendPayAddr asserts that we permit duplicate payment
// addresses to be inserted if they are blank to support JIT legacy keysend
// invoices.
func TestAddDuplicateKeysendPayAddr(t *testing.T) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	require.NoError(t, err)

	// Create two invoices with the same _blank_ payment addr.
	invoice1, err := randInvoice(1000)
	require.NoError(t, err)
	invoice1.Terms.PaymentAddr = BlankPayAddr

	invoice2, err := randInvoice(20000)
	require.NoError(t, err)
	invoice2.Terms.PaymentAddr = BlankPayAddr

	// Inserting both should succeed without a duplicate payment address
	// failure.
	inv1Hash := invoice1.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice1, inv1Hash)
	require.NoError(t, err)

	inv2Hash := invoice2.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice2, inv2Hash)
	require.NoError(t, err)

	// Querying for each should succeed. Here we use hash+addr refs since
	// the lookup will fail if the hash and addr point to different
	// invoices, so if both succeed we can be assured they aren't included
	// in the payment address index.
	ref1 := InvoiceRefByHashAndAddr(inv1Hash, BlankPayAddr)
	dbInv1, err := db.LookupInvoice(ref1)
	require.NoError(t, err)
	require.Equal(t, invoice1, &dbInv1)

	ref2 := InvoiceRefByHashAndAddr(inv2Hash, BlankPayAddr)
	dbInv2, err := db.LookupInvoice(ref2)
	require.NoError(t, err)
	require.Equal(t, invoice2, &dbInv2)
}

// TestFailInvoiceLookupMPPPayAddrOnly asserts that looking up a MPP invoice
// that matches _only_ by payment address fails with ErrInvoiceNotFound. This
// ensures that the HTLC's payment hash always matches the payment hash in the
// returned invoice.
func TestFailInvoiceLookupMPPPayAddrOnly(t *testing.T) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	require.NoError(t, err)

	// Create and insert a random invoice.
	invoice, err := randInvoice(1000)
	require.NoError(t, err)

	payHash := invoice.Terms.PaymentPreimage.Hash()
	payAddr := invoice.Terms.PaymentAddr
	_, err = db.AddInvoice(invoice, payHash)
	require.NoError(t, err)

	// Modify the queried payment hash to be invalid.
	payHash[0] ^= 0x01

	// Lookup the invoice by (invalid) payment hash and payment address. The
	// lookup should fail since we require the payment hash to match for
	// legacy/MPP invoices, as this guarantees that the preimage is valid
	// for the given HTLC.
	ref := InvoiceRefByHashAndAddr(payHash, payAddr)
	_, err = db.LookupInvoice(ref)
	require.Equal(t, ErrInvoiceNotFound, err)
}

// TestInvRefEquivocation asserts that retrieving or updating an invoice using
// an equivocating InvoiceRef results in ErrInvRefEquivocation.
func TestInvRefEquivocation(t *testing.T) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	require.NoError(t, err)

	// Add two random invoices.
	invoice1, err := randInvoice(1000)
	require.NoError(t, err)

	inv1Hash := invoice1.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice1, inv1Hash)
	require.NoError(t, err)

	invoice2, err := randInvoice(2000)
	require.NoError(t, err)

	inv2Hash := invoice2.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice2, inv2Hash)
	require.NoError(t, err)

	// Now, query using invoice 1's payment address, but invoice 2's payment
	// hash. We expect an error since the invref points to multiple
	// invoices.
	ref := InvoiceRefByHashAndAddr(inv2Hash, invoice1.Terms.PaymentAddr)
	_, err = db.LookupInvoice(ref)
	require.Error(t, err, ErrInvRefEquivocation)

	// The same error should be returned when updating an equivocating
	// reference.
	nop := func(_ *Invoice) (*InvoiceUpdateDesc, error) {
		return nil, nil
	}
	_, err = db.UpdateInvoice(ref, nop)
	require.Error(t, err, ErrInvRefEquivocation)
}

// TestInvoiceCancelSingleHtlc tests that a single htlc can be canceled on the
// invoice.
func TestInvoiceCancelSingleHtlc(t *testing.T) {
	t.Parallel()

	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	preimage := lntypes.Preimage{1}
	paymentHash := preimage.Hash()

	testInvoice := &Invoice{
		Htlcs: map[CircuitKey]*InvoiceHTLC{},
		Terms: ContractTerm{
			Value:           lnwire.NewMSatFromSatoshis(10000),
			Features:        emptyFeatures,
			PaymentPreimage: &preimage,
		},
	}

	if _, err := db.AddInvoice(testInvoice, paymentHash); err != nil {
		t.Fatalf("unable to find invoice: %v", err)
	}

	// Accept an htlc on this invoice.
	key := CircuitKey{ChanID: lnwire.NewShortChanIDFromInt(1), HtlcID: 4}
	htlc := HtlcAcceptDesc{
		Amt:           500,
		CustomRecords: make(record.CustomSet),
	}

	ref := InvoiceRefByHash(paymentHash)
	invoice, err := db.UpdateInvoice(ref,
		func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
			return &InvoiceUpdateDesc{
				AddHtlcs: map[CircuitKey]*HtlcAcceptDesc{
					key: &htlc,
				},
			}, nil
		})
	if err != nil {
		t.Fatalf("unable to add invoice htlc: %v", err)
	}
	if len(invoice.Htlcs) != 1 {
		t.Fatalf("expected the htlc to be added")
	}
	if invoice.Htlcs[key].State != HtlcStateAccepted {
		t.Fatalf("expected htlc in state accepted")
	}

	// Cancel the htlc again.
	invoice, err = db.UpdateInvoice(ref,
		func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
			return &InvoiceUpdateDesc{
				CancelHtlcs: map[CircuitKey]struct{}{
					key: {},
				},
			}, nil
		})
	if err != nil {
		t.Fatalf("unable to cancel htlc: %v", err)
	}
	if len(invoice.Htlcs) != 1 {
		t.Fatalf("expected the htlc to be present")
	}
	if invoice.Htlcs[key].State != HtlcStateCanceled {
		t.Fatalf("expected htlc in state canceled")
	}
}

// TestInvoiceTimeSeries tests that newly added invoices invoices, as well as
// settled invoices are added to the database are properly placed in the add
// add or settle index which serves as an event time series.
func TestInvoiceAddTimeSeries(t *testing.T) {
	t.Parallel()

	db, cleanUp, err := MakeTestDB(OptionClock(testClock))
	defer cleanUp()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	_, err = db.InvoicesAddedSince(0)
	require.NoError(t, err)

	// We'll start off by creating 20 random invoices, and inserting them
	// into the database.
	const numInvoices = 20
	amt := lnwire.NewMSatFromSatoshis(1000)
	invoices := make([]Invoice, numInvoices)
	for i := 0; i < len(invoices); i++ {
		invoice, err := randInvoice(amt)
		if err != nil {
			t.Fatalf("unable to create invoice: %v", err)
		}

		paymentHash := invoice.Terms.PaymentPreimage.Hash()

		if _, err := db.AddInvoice(invoice, paymentHash); err != nil {
			t.Fatalf("unable to add invoice %v", err)
		}

		invoices[i] = *invoice
	}

	// With the invoices constructed, we'll now create a series of queries
	// that we'll use to assert expected return values of
	// InvoicesAddedSince.
	addQueries := []struct {
		sinceAddIndex uint64

		resp []Invoice
	}{
		// If we specify a value of zero, we shouldn't get any invoices
		// back.
		{
			sinceAddIndex: 0,
		},

		// If we specify a value well beyond the number of inserted
		// invoices, we shouldn't get any invoices back.
		{
			sinceAddIndex: 99999999,
		},

		// Using an index of 1 should result in all values, but the
		// first one being returned.
		{
			sinceAddIndex: 1,
			resp:          invoices[1:],
		},

		// If we use an index of 10, then we should retrieve the
		// reaming 10 invoices.
		{
			sinceAddIndex: 10,
			resp:          invoices[10:],
		},
	}

	for i, query := range addQueries {
		resp, err := db.InvoicesAddedSince(query.sinceAddIndex)
		if err != nil {
			t.Fatalf("unable to query: %v", err)
		}

		require.Equal(t, len(query.resp), len(resp))

		for j := 0; j < len(query.resp); j++ {
			require.Equal(t,
				query.resp[j], resp[j],
				fmt.Sprintf("test: #%v, item: #%v", i, j),
			)
		}
	}

	_, err = db.InvoicesSettledSince(0)
	require.NoError(t, err)

	var settledInvoices []Invoice
	var settleIndex uint64 = 1
	// We'll now only settle the latter half of each of those invoices.
	for i := 10; i < len(invoices); i++ {
		invoice := &invoices[i]

		paymentHash := invoice.Terms.PaymentPreimage.Hash()

		ref := InvoiceRefByHash(paymentHash)
		_, err := db.UpdateInvoice(
			ref, getUpdateInvoice(invoice.Terms.Value),
		)
		if err != nil {
			t.Fatalf("unable to settle invoice: %v", err)
		}

		// Create the settled invoice for the expectation set.
		settleTestInvoice(invoice, settleIndex)
		settleIndex++

		settledInvoices = append(settledInvoices, *invoice)
	}

	// We'll now prepare an additional set of queries to ensure the settle
	// time series has properly been maintained in the database.
	settleQueries := []struct {
		sinceSettleIndex uint64

		resp []Invoice
	}{
		// If we specify a value of zero, we shouldn't get any settled
		// invoices back.
		{
			sinceSettleIndex: 0,
		},

		// If we specify a value well beyond the number of settled
		// invoices, we shouldn't get any invoices back.
		{
			sinceSettleIndex: 99999999,
		},

		// Using an index of 1 should result in the final 10 invoices
		// being returned, as we only settled those.
		{
			sinceSettleIndex: 1,
			resp:             settledInvoices[1:],
		},
	}

	for i, query := range settleQueries {
		resp, err := db.InvoicesSettledSince(query.sinceSettleIndex)
		if err != nil {
			t.Fatalf("unable to query: %v", err)
		}

		require.Equal(t, len(query.resp), len(resp))

		for j := 0; j < len(query.resp); j++ {
			require.Equal(t,
				query.resp[j], resp[j],
				fmt.Sprintf("test: #%v, item: #%v", i, j),
			)
		}
	}
}

// TestScanInvoices tests that ScanInvoices scans trough all stored invoices
// correctly.
func TestScanInvoices(t *testing.T) {
	t.Parallel()

	db, cleanup, err := MakeTestDB()
	defer cleanup()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	var invoices map[lntypes.Hash]*Invoice
	callCount := 0
	resetCount := 0

	// reset is used to reset/initialize results and is called once
	// upon calling ScanInvoices and when the underlying transaction is
	// retried.
	reset := func() {
		invoices = make(map[lntypes.Hash]*Invoice)
		callCount = 0
		resetCount++

	}

	scanFunc := func(paymentHash lntypes.Hash, invoice *Invoice) error {
		invoices[paymentHash] = invoice
		callCount++

		return nil
	}

	// With an empty DB we expect to not scan any invoices.
	require.NoError(t, db.ScanInvoices(scanFunc, reset))
	require.Equal(t, 0, len(invoices))
	require.Equal(t, 0, callCount)
	require.Equal(t, 1, resetCount)

	numInvoices := 5
	testInvoices := make(map[lntypes.Hash]*Invoice)

	// Now populate the DB and check if we can get all invoices with their
	// payment hashes as expected.
	for i := 1; i <= numInvoices; i++ {
		invoice, err := randInvoice(lnwire.MilliSatoshi(i))
		require.NoError(t, err)

		paymentHash := invoice.Terms.PaymentPreimage.Hash()
		testInvoices[paymentHash] = invoice

		_, err = db.AddInvoice(invoice, paymentHash)
		require.NoError(t, err)
	}

	resetCount = 0
	require.NoError(t, db.ScanInvoices(scanFunc, reset))
	require.Equal(t, numInvoices, callCount)
	require.Equal(t, testInvoices, invoices)
	require.Equal(t, 1, resetCount)
}

// TestDuplicateSettleInvoice tests that if we add a new invoice and settle it
// twice, then the second time we also receive the invoice that we settled as a
// return argument.
func TestDuplicateSettleInvoice(t *testing.T) {
	t.Parallel()

	db, cleanUp, err := MakeTestDB(OptionClock(testClock))
	defer cleanUp()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	// We'll start out by creating an invoice and writing it to the DB.
	amt := lnwire.NewMSatFromSatoshis(1000)
	invoice, err := randInvoice(amt)
	if err != nil {
		t.Fatalf("unable to create invoice: %v", err)
	}

	payHash := invoice.Terms.PaymentPreimage.Hash()

	if _, err := db.AddInvoice(invoice, payHash); err != nil {
		t.Fatalf("unable to add invoice %v", err)
	}

	// With the invoice in the DB, we'll now attempt to settle the invoice.
	ref := InvoiceRefByHash(payHash)
	dbInvoice, err := db.UpdateInvoice(ref, getUpdateInvoice(amt))
	if err != nil {
		t.Fatalf("unable to settle invoice: %v", err)
	}

	// We'll update what we expect the settle invoice to be so that our
	// comparison below has the correct assumption.
	invoice.SettleIndex = 1
	invoice.State = ContractSettled
	invoice.AmtPaid = amt
	invoice.SettleDate = dbInvoice.SettleDate
	invoice.Htlcs = map[CircuitKey]*InvoiceHTLC{
		{}: {
			Amt:           amt,
			AcceptTime:    time.Unix(1, 0),
			ResolveTime:   time.Unix(1, 0),
			State:         HtlcStateSettled,
			CustomRecords: make(record.CustomSet),
		},
	}

	// We should get back the exact same invoice that we just inserted.
	require.Equal(t, invoice, dbInvoice, "wrong invoice after settle")

	// If we try to settle the invoice again, then we should get the very
	// same invoice back, but with an error this time.
	dbInvoice, err = db.UpdateInvoice(ref, getUpdateInvoice(amt))
	if err != ErrInvoiceAlreadySettled {
		t.Fatalf("expected ErrInvoiceAlreadySettled")
	}

	if dbInvoice == nil {
		t.Fatalf("invoice from db is nil after settle!")
	}

	invoice.SettleDate = dbInvoice.SettleDate
	require.Equal(t, invoice, dbInvoice, "wrong invoice after second settle")
}

// TestQueryInvoices ensures that we can properly query the invoice database for
// invoices using different types of queries.
func TestQueryInvoices(t *testing.T) {
	t.Parallel()

	db, cleanUp, err := MakeTestDB(OptionClock(testClock))
	defer cleanUp()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	// To begin the test, we'll add 50 invoices to the database. We'll
	// assume that the index of the invoice within the database is the same
	// as the amount of the invoice itself.
	const numInvoices = 50
	var settleIndex uint64 = 1
	var invoices []Invoice
	var pendingInvoices []Invoice

	for i := 1; i <= numInvoices; i++ {
		amt := lnwire.MilliSatoshi(i)
		invoice, err := randInvoice(amt)
		if err != nil {
			t.Fatalf("unable to create invoice: %v", err)
		}

		paymentHash := invoice.Terms.PaymentPreimage.Hash()

		if _, err := db.AddInvoice(invoice, paymentHash); err != nil {
			t.Fatalf("unable to add invoice: %v", err)
		}

		// We'll only settle half of all invoices created.
		if i%2 == 0 {
			ref := InvoiceRefByHash(paymentHash)
			_, err := db.UpdateInvoice(ref, getUpdateInvoice(amt))
			if err != nil {
				t.Fatalf("unable to settle invoice: %v", err)
			}

			// Create the settled invoice for the expectation set.
			settleTestInvoice(invoice, settleIndex)
			settleIndex++
		} else {
			pendingInvoices = append(pendingInvoices, *invoice)
		}

		invoices = append(invoices, *invoice)
	}

	// The test will consist of several queries along with their respective
	// expected response. Each query response should match its expected one.
	testCases := []struct {
		query    InvoiceQuery
		expected []Invoice
	}{
		// Fetch all invoices with a single query.
		{
			query: InvoiceQuery{
				NumMaxInvoices: numInvoices,
			},
			expected: invoices,
		},
		// Fetch all invoices with a single query, reversed.
		{
			query: InvoiceQuery{
				Reversed:       true,
				NumMaxInvoices: numInvoices,
			},
			expected: invoices,
		},
		// Fetch the first 25 invoices.
		{
			query: InvoiceQuery{
				NumMaxInvoices: numInvoices / 2,
			},
			expected: invoices[:numInvoices/2],
		},
		// Fetch the first 10 invoices, but this time iterating
		// backwards.
		{
			query: InvoiceQuery{
				IndexOffset:    11,
				Reversed:       true,
				NumMaxInvoices: numInvoices,
			},
			expected: invoices[:10],
		},
		// Fetch the last 40 invoices.
		{
			query: InvoiceQuery{
				IndexOffset:    10,
				NumMaxInvoices: numInvoices,
			},
			expected: invoices[10:],
		},
		// Fetch all but the first invoice.
		{
			query: InvoiceQuery{
				IndexOffset:    1,
				NumMaxInvoices: numInvoices,
			},
			expected: invoices[1:],
		},
		// Fetch one invoice, reversed, with index offset 3. This
		// should give us the second invoice in the array.
		{
			query: InvoiceQuery{
				IndexOffset:    3,
				Reversed:       true,
				NumMaxInvoices: 1,
			},
			expected: invoices[1:2],
		},
		// Same as above, at index 2.
		{
			query: InvoiceQuery{
				IndexOffset:    2,
				Reversed:       true,
				NumMaxInvoices: 1,
			},
			expected: invoices[0:1],
		},
		// Fetch one invoice, at index 1, reversed. Since invoice#1 is
		// the very first, there won't be any left in a reverse search,
		// so we expect no invoices to be returned.
		{
			query: InvoiceQuery{
				IndexOffset:    1,
				Reversed:       true,
				NumMaxInvoices: 1,
			},
			expected: nil,
		},
		// Same as above, but don't restrict the number of invoices to
		// 1.
		{
			query: InvoiceQuery{
				IndexOffset:    1,
				Reversed:       true,
				NumMaxInvoices: numInvoices,
			},
			expected: nil,
		},
		// Fetch one invoice, reversed, with no offset set. We expect
		// the last invoice in the response.
		{
			query: InvoiceQuery{
				Reversed:       true,
				NumMaxInvoices: 1,
			},
			expected: invoices[numInvoices-1:],
		},
		// Fetch one invoice, reversed, the offset set at numInvoices+1.
		// We expect this to return the last invoice.
		{
			query: InvoiceQuery{
				IndexOffset:    numInvoices + 1,
				Reversed:       true,
				NumMaxInvoices: 1,
			},
			expected: invoices[numInvoices-1:],
		},
		// Same as above, at offset numInvoices.
		{
			query: InvoiceQuery{
				IndexOffset:    numInvoices,
				Reversed:       true,
				NumMaxInvoices: 1,
			},
			expected: invoices[numInvoices-2 : numInvoices-1],
		},
		// Fetch one invoice, at no offset (same as offset 0). We
		// expect the first invoice only in the response.
		{
			query: InvoiceQuery{
				NumMaxInvoices: 1,
			},
			expected: invoices[:1],
		},
		// Same as above, at offset 1.
		{
			query: InvoiceQuery{
				IndexOffset:    1,
				NumMaxInvoices: 1,
			},
			expected: invoices[1:2],
		},
		// Same as above, at offset 2.
		{
			query: InvoiceQuery{
				IndexOffset:    2,
				NumMaxInvoices: 1,
			},
			expected: invoices[2:3],
		},
		// Same as above, at offset numInvoices-1. Expect the last
		// invoice to be returned.
		{
			query: InvoiceQuery{
				IndexOffset:    numInvoices - 1,
				NumMaxInvoices: 1,
			},
			expected: invoices[numInvoices-1:],
		},
		// Same as above, at offset numInvoices. No invoices should be
		// returned, as there are no invoices after this offset.
		{
			query: InvoiceQuery{
				IndexOffset:    numInvoices,
				NumMaxInvoices: 1,
			},
			expected: nil,
		},
		// Fetch all pending invoices with a single query.
		{
			query: InvoiceQuery{
				PendingOnly:    true,
				NumMaxInvoices: numInvoices,
			},
			expected: pendingInvoices,
		},
		// Fetch the first 12 pending invoices.
		{
			query: InvoiceQuery{
				PendingOnly:    true,
				NumMaxInvoices: numInvoices / 4,
			},
			expected: pendingInvoices[:len(pendingInvoices)/2],
		},
		// Fetch the first 5 pending invoices, but this time iterating
		// backwards.
		{
			query: InvoiceQuery{
				IndexOffset:    10,
				PendingOnly:    true,
				Reversed:       true,
				NumMaxInvoices: numInvoices,
			},
			// Since we seek to the invoice with index 10 and
			// iterate backwards, there should only be 5 pending
			// invoices before it as every other invoice within the
			// index is settled.
			expected: pendingInvoices[:5],
		},
		// Fetch the last 15 invoices.
		{
			query: InvoiceQuery{
				IndexOffset:    20,
				PendingOnly:    true,
				NumMaxInvoices: numInvoices,
			},
			// Since we seek to the invoice with index 20, there are
			// 30 invoices left. From these 30, only 15 of them are
			// still pending.
			expected: pendingInvoices[len(pendingInvoices)-15:],
		},
		// Fetch all invoices paginating backwards, with an index offset
		// that is beyond our last offset. We expect all invoices to be
		// returned.
		{
			query: InvoiceQuery{
				IndexOffset:    numInvoices * 2,
				PendingOnly:    false,
				Reversed:       true,
				NumMaxInvoices: numInvoices,
			},
			expected: invoices,
		},
	}

	for i, testCase := range testCases {
		response, err := db.QueryInvoices(testCase.query)
		if err != nil {
			t.Fatalf("unable to query invoice database: %v", err)
		}

		require.Equal(t, len(testCase.expected), len(response.Invoices))

		for j, expected := range testCase.expected {
			require.Equal(t,
				expected, response.Invoices[j],
				fmt.Sprintf("test: #%v, item: #%v", i, j),
			)
		}
	}
}

// getUpdateInvoice returns an invoice update callback that, when called,
// settles the invoice with the given amount.
func getUpdateInvoice(amt lnwire.MilliSatoshi) InvoiceUpdateCallback {
	return func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
		if invoice.State == ContractSettled {
			return nil, ErrInvoiceAlreadySettled
		}

		noRecords := make(record.CustomSet)

		update := &InvoiceUpdateDesc{
			State: &InvoiceStateUpdateDesc{
				Preimage: invoice.Terms.PaymentPreimage,
				NewState: ContractSettled,
			},
			AddHtlcs: map[CircuitKey]*HtlcAcceptDesc{
				{}: {
					Amt:           amt,
					CustomRecords: noRecords,
				},
			},
		}

		return update, nil
	}
}

// TestCustomRecords tests that custom records are properly recorded in the
// invoice database.
func TestCustomRecords(t *testing.T) {
	t.Parallel()

	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	if err != nil {
		t.Fatalf("unable to make test db: %v", err)
	}

	preimage := lntypes.Preimage{1}
	paymentHash := preimage.Hash()

	testInvoice := &Invoice{
		Htlcs: map[CircuitKey]*InvoiceHTLC{},
		Terms: ContractTerm{
			Value:           lnwire.NewMSatFromSatoshis(10000),
			Features:        emptyFeatures,
			PaymentPreimage: &preimage,
		},
	}

	if _, err := db.AddInvoice(testInvoice, paymentHash); err != nil {
		t.Fatalf("unable to add invoice: %v", err)
	}

	// Accept an htlc with custom records on this invoice.
	key := CircuitKey{ChanID: lnwire.NewShortChanIDFromInt(1), HtlcID: 4}

	records := record.CustomSet{
		100000: []byte{},
		100001: []byte{1, 2},
	}

	ref := InvoiceRefByHash(paymentHash)
	_, err = db.UpdateInvoice(ref,
		func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
			return &InvoiceUpdateDesc{
				AddHtlcs: map[CircuitKey]*HtlcAcceptDesc{
					key: {
						Amt:           500,
						CustomRecords: records,
					},
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("unable to add invoice htlc: %v", err)
	}

	// Retrieve the invoice from that database and verify that the custom
	// records are present.
	dbInvoice, err := db.LookupInvoice(ref)
	if err != nil {
		t.Fatalf("unable to lookup invoice: %v", err)
	}

	if len(dbInvoice.Htlcs) != 1 {
		t.Fatalf("expected the htlc to be added")
	}

	require.Equal(t,
		records, dbInvoice.Htlcs[key].CustomRecords,
		"invalid custom records",
	)
}

// TestInvoiceHtlcAMPFields asserts that the set id and preimage fields are
// properly recorded when updating an invoice.
func TestInvoiceHtlcAMPFields(t *testing.T) {
	t.Run("amp", func(t *testing.T) {
		testInvoiceHtlcAMPFields(t, true)
	})
	t.Run("no amp", func(t *testing.T) {
		testInvoiceHtlcAMPFields(t, false)
	})
}

func testInvoiceHtlcAMPFields(t *testing.T, isAMP bool) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	require.Nil(t, err)

	testInvoice, err := randInvoice(1000)
	require.Nil(t, err)

	payHash := testInvoice.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(testInvoice, payHash)
	require.Nil(t, err)

	// Accept an htlc with custom records on this invoice.
	key := CircuitKey{ChanID: lnwire.NewShortChanIDFromInt(1), HtlcID: 4}
	records := make(map[uint64][]byte)

	var ampData *InvoiceHtlcAMPData
	if isAMP {
		amp := record.NewAMP([32]byte{1}, [32]byte{2}, 3)
		preimage := &lntypes.Preimage{4}

		ampData = &InvoiceHtlcAMPData{
			Record:   *amp,
			Hash:     preimage.Hash(),
			Preimage: preimage,
		}
	}

	ref := InvoiceRefByHash(payHash)
	_, err = db.UpdateInvoice(ref,
		func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
			return &InvoiceUpdateDesc{
				AddHtlcs: map[CircuitKey]*HtlcAcceptDesc{
					key: {
						Amt:           500,
						AMP:           ampData,
						CustomRecords: records,
					},
				},
			}, nil
		},
	)
	require.Nil(t, err)

	// Retrieve the invoice from that database and verify that the AMP
	// fields are as expected.
	dbInvoice, err := db.LookupInvoice(ref)
	require.Nil(t, err)

	require.Equal(t, 1, len(dbInvoice.Htlcs))
	require.Equal(t, ampData, dbInvoice.Htlcs[key].AMP)
}

// TestInvoiceRef asserts that the proper identifiers are returned from an
// InvoiceRef depending on the constructor used.
func TestInvoiceRef(t *testing.T) {
	payHash := lntypes.Hash{0x01}
	payAddr := [32]byte{0x02}
	setID := [32]byte{0x03}

	// An InvoiceRef by hash should return the provided hash and a nil
	// payment addr.
	refByHash := InvoiceRefByHash(payHash)
	require.Equal(t, &payHash, refByHash.PayHash())
	require.Equal(t, (*[32]byte)(nil), refByHash.PayAddr())
	require.Equal(t, (*[32]byte)(nil), refByHash.SetID())

	// An InvoiceRef by hash and addr should return the payment hash and
	// payment addr passed to the constructor.
	refByHashAndAddr := InvoiceRefByHashAndAddr(payHash, payAddr)
	require.Equal(t, &payHash, refByHashAndAddr.PayHash())
	require.Equal(t, &payAddr, refByHashAndAddr.PayAddr())
	require.Equal(t, (*[32]byte)(nil), refByHashAndAddr.SetID())

	// An InvoiceRef by set id should return an empty pay hash, a nil pay
	// addr, and a reference to the given set id.
	refBySetID := InvoiceRefBySetID(setID)
	require.Equal(t, (*lntypes.Hash)(nil), refBySetID.PayHash())
	require.Equal(t, (*[32]byte)(nil), refBySetID.PayAddr())
	require.Equal(t, &setID, refBySetID.SetID())

	// An InvoiceRef by pay addr should only return a pay addr, but nil for
	// pay hash and set id.
	refByAddr := InvoiceRefByAddr(payAddr)
	require.Equal(t, (*lntypes.Hash)(nil), refByAddr.PayHash())
	require.Equal(t, &payAddr, refByAddr.PayAddr())
	require.Equal(t, (*[32]byte)(nil), refByAddr.SetID())
}

// TestHTLCSet asserts that HTLCSet returns the proper set of accepted HTLCs
// that can be considered for settlement. It asserts that MPP and AMP HTLCs do
// not comingle, and also that HTLCs with disjoint set ids appear in different
// sets.
func TestHTLCSet(t *testing.T) {
	inv := &Invoice{
		Htlcs: make(map[CircuitKey]*InvoiceHTLC),
	}

	// Construct two distinct set id's, in this test we'll also track the
	// nil set id as a third group.
	setID1 := &[32]byte{1}
	setID2 := &[32]byte{2}

	// Create the expected htlc sets for each group, these will be updated
	// as the invoice is modified.
	expSetNil := make(map[CircuitKey]*InvoiceHTLC)
	expSet1 := make(map[CircuitKey]*InvoiceHTLC)
	expSet2 := make(map[CircuitKey]*InvoiceHTLC)

	checkHTLCSets := func() {
		require.Equal(t, expSetNil, inv.HTLCSet(nil, HtlcStateAccepted))
		require.Equal(t, expSet1, inv.HTLCSet(setID1, HtlcStateAccepted))
		require.Equal(t, expSet2, inv.HTLCSet(setID2, HtlcStateAccepted))
	}

	// All HTLC sets should be empty initially.
	checkHTLCSets()

	// Add the following sequence of HTLCs to the invoice, sanity checking
	// all three HTLC sets after each transition. This sequence asserts:
	//   - both nil and non-nil set ids can have multiple htlcs.
	//   - there may be distinct htlc sets with non-nil set ids.
	//   - only accepted htlcs are returned as part of the set.
	htlcs := []struct {
		setID *[32]byte
		state HtlcState
	}{
		{nil, HtlcStateAccepted},
		{nil, HtlcStateAccepted},
		{setID1, HtlcStateAccepted},
		{setID1, HtlcStateAccepted},
		{setID2, HtlcStateAccepted},
		{setID2, HtlcStateAccepted},
		{nil, HtlcStateCanceled},
		{setID1, HtlcStateCanceled},
		{setID2, HtlcStateCanceled},
		{nil, HtlcStateSettled},
		{setID1, HtlcStateSettled},
		{setID2, HtlcStateSettled},
	}

	for i, h := range htlcs {
		var ampData *InvoiceHtlcAMPData
		if h.setID != nil {
			ampData = &InvoiceHtlcAMPData{
				Record: *record.NewAMP([32]byte{0}, *h.setID, 0),
			}

		}

		// Add the HTLC to the invoice's set of HTLCs.
		key := CircuitKey{HtlcID: uint64(i)}
		htlc := &InvoiceHTLC{
			AMP:   ampData,
			State: h.state,
		}
		inv.Htlcs[key] = htlc

		// Update our expected htlc set if the htlc is accepted,
		// otherwise it shouldn't be reflected.
		if h.state == HtlcStateAccepted {
			switch h.setID {
			case nil:
				expSetNil[key] = htlc
			case setID1:
				expSet1[key] = htlc
			case setID2:
				expSet2[key] = htlc
			default:
				t.Fatalf("unexpected set id")
			}
		}

		checkHTLCSets()
	}
}

// TestAddInvoiceWithHTLCs asserts that you can't insert an invoice that already
// has HTLCs.
func TestAddInvoiceWithHTLCs(t *testing.T) {
	db, cleanUp, err := MakeTestDB()
	defer cleanUp()
	require.Nil(t, err)

	testInvoice, err := randInvoice(1000)
	require.Nil(t, err)

	key := CircuitKey{HtlcID: 1}
	testInvoice.Htlcs[key] = &InvoiceHTLC{}

	payHash := testInvoice.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(testInvoice, payHash)
	require.Equal(t, ErrInvoiceHasHtlcs, err)
}

// TestSetIDIndex asserts that the set id index properly adds new invoices as we
// accept HTLCs, that they can be queried by their set id after accepting, and
// that invoices with duplicate set ids are disallowed.
func TestSetIDIndex(t *testing.T) {
	testClock := clock.NewTestClock(testNow)
	db, cleanUp, err := MakeTestDB(OptionClock(testClock))
	defer cleanUp()
	require.Nil(t, err)

	// We'll start out by creating an invoice and writing it to the DB.
	amt := lnwire.NewMSatFromSatoshis(1000)
	invoice, err := randInvoice(amt)
	require.Nil(t, err)

	preimage := *invoice.Terms.PaymentPreimage
	payHash := preimage.Hash()
	_, err = db.AddInvoice(invoice, payHash)
	require.Nil(t, err)

	setID := &[32]byte{1}

	// Update the invoice with an accepted HTLC that also accepts the
	// invoice.
	ref := InvoiceRefByHashAndAddr(payHash, invoice.Terms.PaymentAddr)
	dbInvoice, err := db.UpdateInvoice(ref, updateAcceptAMPHtlc(0, amt, setID, true))
	require.Nil(t, err)

	// We'll update what we expect the accepted invoice to be so that our
	// comparison below has the correct assumption.
	invoice.State = ContractAccepted
	invoice.AmtPaid = amt
	invoice.SettleDate = dbInvoice.SettleDate
	invoice.Htlcs = map[CircuitKey]*InvoiceHTLC{
		{HtlcID: 0}: makeAMPInvoiceHTLC(amt, *setID, payHash, &preimage),
	}

	// We should get back the exact same invoice that we just inserted.
	require.Equal(t, invoice, dbInvoice)

	// Now lookup the invoice by set id and see that we get the same one.
	refBySetID := InvoiceRefBySetID(*setID)
	dbInvoiceBySetID, err := db.LookupInvoice(refBySetID)
	require.Nil(t, err)
	require.Equal(t, invoice, &dbInvoiceBySetID)

	// Trying to accept an HTLC to a different invoice, but using the same
	// set id should fail.
	invoice2, err := randInvoice(amt)
	require.Nil(t, err)

	payHash2 := invoice2.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice2, payHash2)
	require.Nil(t, err)

	ref2 := InvoiceRefByHashAndAddr(payHash2, invoice2.Terms.PaymentAddr)
	_, err = db.UpdateInvoice(ref2, updateAcceptAMPHtlc(0, amt, setID, true))
	require.Equal(t, ErrDuplicateSetID{setID: *setID}, err)

	// Now, begin constructing a second htlc set under a different set id.
	// This set will contain two distinct HTLCs.
	setID2 := &[32]byte{2}

	_, err = db.UpdateInvoice(ref, updateAcceptAMPHtlc(1, amt, setID2, false))
	require.Nil(t, err)
	dbInvoice, err = db.UpdateInvoice(ref, updateAcceptAMPHtlc(2, amt, setID2, false))
	require.Nil(t, err)

	// We'll update what we expect the settle invoice to be so that our
	// comparison below has the correct assumption.
	invoice.State = ContractAccepted
	invoice.AmtPaid += 2 * amt
	invoice.SettleDate = dbInvoice.SettleDate
	invoice.Htlcs = map[CircuitKey]*InvoiceHTLC{
		{HtlcID: 0}: makeAMPInvoiceHTLC(amt, *setID, payHash, &preimage),
		{HtlcID: 1}: makeAMPInvoiceHTLC(amt, *setID2, payHash, nil),
		{HtlcID: 2}: makeAMPInvoiceHTLC(amt, *setID2, payHash, nil),
	}

	// We should get back the exact same invoice that we just inserted.
	require.Equal(t, invoice, dbInvoice)

	// Now lookup the invoice by second set id and see that we get the same
	// index, including the htlcs under the first set id.
	refBySetID = InvoiceRefBySetID(*setID2)
	dbInvoiceBySetID, err = db.LookupInvoice(refBySetID)
	require.Nil(t, err)
	require.Equal(t, invoice, &dbInvoiceBySetID)

	// Now settle the first htlc set, asserting that the two htlcs with set
	// id 2 get canceled as a result.
	_, err = db.UpdateInvoice(
		ref, getUpdateInvoiceAMPSettle(&[32]byte{}),
	)
	require.Equal(t, ErrEmptyHTLCSet, err)

	// Now settle the first htlc set, asserting that the two htlcs with set
	// id 2 get canceled as a result.
	dbInvoice, err = db.UpdateInvoice(ref, getUpdateInvoiceAMPSettle(setID))
	require.Nil(t, err)

	invoice.State = ContractSettled
	invoice.SettleDate = dbInvoice.SettleDate
	invoice.SettleIndex = 1
	invoice.AmtPaid = amt
	invoice.Htlcs[CircuitKey{HtlcID: 0}].ResolveTime = time.Unix(1, 0)
	invoice.Htlcs[CircuitKey{HtlcID: 0}].State = HtlcStateSettled
	invoice.Htlcs[CircuitKey{HtlcID: 1}].ResolveTime = time.Unix(1, 0)
	invoice.Htlcs[CircuitKey{HtlcID: 1}].State = HtlcStateCanceled
	invoice.Htlcs[CircuitKey{HtlcID: 2}].ResolveTime = time.Unix(1, 0)
	invoice.Htlcs[CircuitKey{HtlcID: 2}].State = HtlcStateCanceled
	require.Equal(t, invoice, dbInvoice)

	// Lastly, querying for an unknown set id should fail.
	refUnknownSetID := InvoiceRefBySetID([32]byte{})
	_, err = db.LookupInvoice(refUnknownSetID)
	require.Equal(t, ErrInvoiceNotFound, err)
}

func makeAMPInvoiceHTLC(amt lnwire.MilliSatoshi, setID [32]byte,
	hash lntypes.Hash, preimage *lntypes.Preimage) *InvoiceHTLC {

	return &InvoiceHTLC{
		Amt:           amt,
		AcceptTime:    testNow,
		ResolveTime:   time.Time{},
		State:         HtlcStateAccepted,
		CustomRecords: make(record.CustomSet),
		AMP: &InvoiceHtlcAMPData{
			Record:   *record.NewAMP([32]byte{}, setID, 0),
			Hash:     hash,
			Preimage: preimage,
		},
	}
}

// updateAcceptAMPHtlc returns an invoice update callback that, when called,
// settles the invoice with the given amount.
func updateAcceptAMPHtlc(id uint64, amt lnwire.MilliSatoshi,
	setID *[32]byte, accept bool) InvoiceUpdateCallback {

	return func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
		if invoice.State == ContractSettled {
			return nil, ErrInvoiceAlreadySettled
		}

		noRecords := make(record.CustomSet)

		var (
			state    *InvoiceStateUpdateDesc
			preimage *lntypes.Preimage
		)
		if accept {
			state = &InvoiceStateUpdateDesc{
				NewState: ContractAccepted,
				SetID:    setID,
			}
			pre := *invoice.Terms.PaymentPreimage
			preimage = &pre
		}

		ampData := &InvoiceHtlcAMPData{
			Record:   *record.NewAMP([32]byte{}, *setID, 0),
			Hash:     invoice.Terms.PaymentPreimage.Hash(),
			Preimage: preimage,
		}
		update := &InvoiceUpdateDesc{
			State: state,
			AddHtlcs: map[CircuitKey]*HtlcAcceptDesc{
				{HtlcID: id}: {
					Amt:           amt,
					CustomRecords: noRecords,
					AMP:           ampData,
				},
			},
		}

		return update, nil
	}
}

func getUpdateInvoiceAMPSettle(setID *[32]byte) InvoiceUpdateCallback {
	return func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
		if invoice.State == ContractSettled {
			return nil, ErrInvoiceAlreadySettled
		}

		update := &InvoiceUpdateDesc{
			State: &InvoiceStateUpdateDesc{
				Preimage: nil,
				NewState: ContractSettled,
				SetID:    setID,
			},
		}

		return update, nil
	}
}

// TestUnexpectedInvoicePreimage asserts that legacy or MPP invoices cannot be
// settled when referenced by payment address only. Since regular or MPP
// payments do not store the payment hash explicitly (it is stored in the
// index), this enforces that they can only be updated using a InvoiceRefByHash
// or InvoiceRefByHashOrAddr.
func TestUnexpectedInvoicePreimage(t *testing.T) {
	t.Parallel()

	db, cleanup, err := MakeTestDB()
	defer cleanup()
	require.NoError(t, err, "unable to make test db")

	invoice, err := randInvoice(lnwire.MilliSatoshi(100))
	require.NoError(t, err)

	// Add a random invoice indexed by payment hash and payment addr.
	paymentHash := invoice.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice, paymentHash)
	require.NoError(t, err)

	// Attempt to update the invoice by pay addr only. This will fail since,
	// in order to settle an MPP invoice, the InvoiceRef must present a
	// payment hash against which to validate the preimage.
	_, err = db.UpdateInvoice(
		InvoiceRefByAddr(invoice.Terms.PaymentAddr),
		getUpdateInvoice(invoice.Terms.Value),
	)

	//Assert that we get ErrUnexpectedInvoicePreimage.
	require.Error(t, ErrUnexpectedInvoicePreimage, err)
}

type updateHTLCPreimageTestCase struct {
	name               string
	settleSamePreimage bool
	expError           error
}

// TestUpdateHTLCPreimages asserts various properties of setting HTLC-level
// preimages on invoice state transitions.
func TestUpdateHTLCPreimages(t *testing.T) {
	t.Parallel()

	tests := []updateHTLCPreimageTestCase{
		{
			name:               "same preimage on settle",
			settleSamePreimage: true,
			expError:           nil,
		},
		{
			name:               "diff preimage on settle",
			settleSamePreimage: false,
			expError:           ErrHTLCPreimageAlreadyExists,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			testUpdateHTLCPreimages(t, test)
		})
	}
}

func testUpdateHTLCPreimages(t *testing.T, test updateHTLCPreimageTestCase) {
	db, cleanup, err := MakeTestDB()
	defer cleanup()
	require.NoError(t, err, "unable to make test db")

	// We'll start out by creating an invoice and writing it to the DB.
	amt := lnwire.NewMSatFromSatoshis(1000)
	invoice, err := randInvoice(amt)
	require.Nil(t, err)

	preimage := *invoice.Terms.PaymentPreimage
	payHash := preimage.Hash()

	// Set AMP-specific features so that we can settle with HTLC-level
	// preimages.
	invoice.Terms.Features = ampFeatures

	_, err = db.AddInvoice(invoice, payHash)
	require.Nil(t, err)

	setID := &[32]byte{1}

	// Update the invoice with an accepted HTLC that also accepts the
	// invoice.
	ref := InvoiceRefByAddr(invoice.Terms.PaymentAddr)
	dbInvoice, err := db.UpdateInvoice(
		ref, updateAcceptAMPHtlc(0, amt, setID, true),
	)
	require.Nil(t, err)

	htlcPreimages := make(map[CircuitKey]lntypes.Preimage)
	for key := range dbInvoice.Htlcs {
		// Set the either the same preimage used to accept above, or a
		// blank preimage depending on the test case.
		var pre lntypes.Preimage
		if test.settleSamePreimage {
			pre = preimage
		}
		htlcPreimages[key] = pre
	}

	updateInvoice := func(invoice *Invoice) (*InvoiceUpdateDesc, error) {
		update := &InvoiceUpdateDesc{
			State: &InvoiceStateUpdateDesc{
				Preimage:      nil,
				NewState:      ContractSettled,
				HTLCPreimages: htlcPreimages,
				SetID:         setID,
			},
		}

		return update, nil
	}

	// Now settle the HTLC set and assert the resulting error.
	_, err = db.UpdateInvoice(ref, updateInvoice)
	require.Equal(t, test.expError, err)
}

type updateHTLCTest struct {
	name     string
	input    InvoiceHTLC
	invState ContractState
	setID    *[32]byte
	output   InvoiceHTLC
	expErr   error
}

// TestUpdateHTLC asserts the behavior of the updateHTLC method in various
// scenarios for MPP and AMP.
func TestUpdateHTLC(t *testing.T) {
	t.Parallel()

	setID := [32]byte{0x01}
	ampRecord := record.NewAMP([32]byte{0x02}, setID, 3)
	preimage := lntypes.Preimage{0x04}
	hash := preimage.Hash()

	diffSetID := [32]byte{0x05}
	fakePreimage := lntypes.Preimage{0x06}
	testAlreadyNow := time.Now()

	tests := []updateHTLCTest{
		{
			name: "MPP accept",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP:           nil,
			},
			invState: ContractAccepted,
			setID:    nil,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP:           nil,
			},
			expErr: nil,
		},
		{
			name: "MPP settle",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP:           nil,
			},
			invState: ContractSettled,
			setID:    nil,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP:           nil,
			},
			expErr: nil,
		},
		{
			name: "MPP cancel",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP:           nil,
			},
			invState: ContractCanceled,
			setID:    nil,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP:           nil,
			},
			expErr: nil,
		},
		{
			name: "AMP accept missing preimage",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: nil,
				},
			},
			invState: ContractAccepted,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: nil,
				},
			},
			expErr: ErrHTLCPreimageMissing,
		},
		{
			name: "AMP accept invalid preimage",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &fakePreimage,
				},
			},
			invState: ContractAccepted,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &fakePreimage,
				},
			},
			expErr: ErrHTLCPreimageMismatch,
		},
		{
			name: "AMP accept valid preimage",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractAccepted,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "AMP accept valid preimage different htlc set",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractAccepted,
			setID:    &diffSetID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "AMP settle missing preimage",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: nil,
				},
			},
			invState: ContractSettled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: nil,
				},
			},
			expErr: ErrHTLCPreimageMissing,
		},
		{
			name: "AMP settle invalid preimage",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &fakePreimage,
				},
			},
			invState: ContractSettled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &fakePreimage,
				},
			},
			expErr: ErrHTLCPreimageMismatch,
		},
		{
			name: "AMP settle valid preimage",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractSettled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			// With the newer AMP logic, this is now valid, as we
			// want to be able to accept multiple settle attempts
			// to a given pay_addr. In this case, the HTLC should
			// remain in the accepted state.
			name: "AMP settle valid preimage different htlc set",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractSettled,
			setID:    &diffSetID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "accept invoice htlc already settled",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractAccepted,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: ErrHTLCAlreadySettled,
		},
		{
			name: "cancel invoice htlc already settled",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractCanceled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: ErrHTLCAlreadySettled,
		},
		{
			name: "settle invoice htlc already settled",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractSettled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateSettled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "cancel invoice",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   time.Time{},
				Expiry:        40,
				State:         HtlcStateAccepted,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractCanceled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "accept invoice htlc already canceled",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractAccepted,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "cancel invoice htlc already canceled",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractCanceled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
		{
			name: "settle invoice htlc already canceled",
			input: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			invState: ContractSettled,
			setID:    &setID,
			output: InvoiceHTLC{
				Amt:           5000,
				MppTotalAmt:   5000,
				AcceptHeight:  100,
				AcceptTime:    testNow,
				ResolveTime:   testAlreadyNow,
				Expiry:        40,
				State:         HtlcStateCanceled,
				CustomRecords: make(record.CustomSet),
				AMP: &InvoiceHtlcAMPData{
					Record:   *ampRecord,
					Hash:     hash,
					Preimage: &preimage,
				},
			},
			expErr: nil,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			testUpdateHTLC(t, test)
		})
	}
}

func testUpdateHTLC(t *testing.T, test updateHTLCTest) {
	htlc := test.input.Copy()
	err := updateHtlc(testNow, htlc, test.invState, test.setID)
	require.Equal(t, test.expErr, err)
	require.Equal(t, test.output, *htlc)
}

// TestDeleteInvoices tests that deleting a list of invoices will succeed
// if all delete references are valid, or will fail otherwise.
func TestDeleteInvoices(t *testing.T) {
	t.Parallel()

	db, cleanup, err := MakeTestDB()
	defer cleanup()
	require.NoError(t, err, "unable to make test db")

	// Add some invoices to the test db.
	numInvoices := 3
	invoicesToDelete := make([]InvoiceDeleteRef, numInvoices)

	for i := 0; i < numInvoices; i++ {
		invoice, err := randInvoice(lnwire.MilliSatoshi(i + 1))
		require.NoError(t, err)

		paymentHash := invoice.Terms.PaymentPreimage.Hash()
		addIndex, err := db.AddInvoice(invoice, paymentHash)
		require.NoError(t, err)

		// Settle the second invoice.
		if i == 1 {
			invoice, err = db.UpdateInvoice(
				InvoiceRefByHash(paymentHash),
				getUpdateInvoice(invoice.Terms.Value),
			)
			require.NoError(t, err, "unable to settle invoice")
		}

		// store the delete ref for later.
		invoicesToDelete[i] = InvoiceDeleteRef{
			PayHash:     paymentHash,
			PayAddr:     &invoice.Terms.PaymentAddr,
			AddIndex:    addIndex,
			SettleIndex: invoice.SettleIndex,
		}
	}

	// assertInvoiceCount asserts that the number of invoices equals
	// to the passed count.
	assertInvoiceCount := func(count int) {
		// Query to collect all invoices.
		query := InvoiceQuery{
			IndexOffset:    0,
			NumMaxInvoices: math.MaxUint64,
		}

		// Check that we really have 3 invoices.
		response, err := db.QueryInvoices(query)
		require.NoError(t, err)
		require.Equal(t, count, len(response.Invoices))
	}

	// XOR one byte of one of the references' hash and attempt to delete.
	invoicesToDelete[0].PayHash[2] ^= 3
	require.Error(t, db.DeleteInvoice(invoicesToDelete))
	assertInvoiceCount(3)

	// Restore the hash.
	invoicesToDelete[0].PayHash[2] ^= 3

	// XOR the second invoice's payment settle index as it is settled, and
	// attempt to delete.
	invoicesToDelete[1].SettleIndex ^= 11
	require.Error(t, db.DeleteInvoice(invoicesToDelete))
	assertInvoiceCount(3)

	// Restore the settle index.
	invoicesToDelete[1].SettleIndex ^= 11

	// XOR the add index for one of the references and attempt to delete.
	invoicesToDelete[2].AddIndex ^= 13
	require.Error(t, db.DeleteInvoice(invoicesToDelete))
	assertInvoiceCount(3)

	// Restore the add index.
	invoicesToDelete[2].AddIndex ^= 13

	// Delete should succeed with all the valid references.
	require.NoError(t, db.DeleteInvoice(invoicesToDelete))
	assertInvoiceCount(0)

}

// TestAddInvoiceInvalidFeatureDeps asserts that inserting an invoice with
// invalid transitive feature dependencies fails with the appropriate error.
func TestAddInvoiceInvalidFeatureDeps(t *testing.T) {
	t.Parallel()

	db, cleanup, err := MakeTestDB()
	require.NoError(t, err, "unable to make test db")
	defer cleanup()

	invoice, err := randInvoice(500)
	require.NoError(t, err)

	invoice.Terms.Features = lnwire.NewFeatureVector(
		lnwire.NewRawFeatureVector(
			lnwire.TLVOnionPayloadOptional,
			lnwire.MPPOptional,
		),
		lnwire.Features,
	)

	hash := invoice.Terms.PaymentPreimage.Hash()
	_, err = db.AddInvoice(invoice, hash)
	require.Error(t, err, feature.NewErrMissingFeatureDep(
		lnwire.PaymentAddrOptional,
	))
}

// TestEncodeDecodeAmpInvoiceState asserts that the nested TLV
// encoding+decoding for the AMPInvoiceState struct works as expected.
func TestEncodeDecodeAmpInvoiceState(t *testing.T) {
	t.Parallel()

	setID1 := [32]byte{1}
	setID2 := [32]byte{2}
	setID3 := [32]byte{3}

	circuitKey1 := CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(1), HtlcID: 1,
	}
	circuitKey2 := CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(2), HtlcID: 2,
	}
	circuitKey3 := CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(2), HtlcID: 3,
	}

	// Make a sample invoice state map that we'll encode then decode to
	// assert equality of.
	ampState := AMPInvoiceState{
		setID1: InvoiceStateAMP{
			State:       HtlcStateSettled,
			SettleDate:  testNow,
			SettleIndex: 1,
			InvoiceKeys: map[CircuitKey]struct{}{
				circuitKey1: struct{}{},
				circuitKey2: struct{}{},
			},
		},
		setID2: InvoiceStateAMP{
			State:       HtlcStateCanceled,
			SettleDate:  testNow,
			SettleIndex: 2,
			InvoiceKeys: map[CircuitKey]struct{}{
				circuitKey1: struct{}{},
			},
		},
		setID3: InvoiceStateAMP{
			State:       HtlcStateAccepted,
			SettleDate:  testNow,
			SettleIndex: 3,
			InvoiceKeys: map[CircuitKey]struct{}{
				circuitKey1: struct{}{},
				circuitKey2: struct{}{},
				circuitKey3: struct{}{},
			},
		},
	}

	// We'll now make a sample invoice stream, and use that to encode the
	// amp state we created above.
	tlvStream, err := tlv.NewStream(
		tlv.MakeDynamicRecord(
			invoiceAmpStateType, &ampState, ampState.recordSize,
			ampStateEncoder, ampStateDecoder,
		),
	)
	require.Nil(t, err)

	// Next encode the stream into a set of raw bytes.
	var b bytes.Buffer
	err = tlvStream.Encode(&b)
	require.Nil(t, err)

	// Now create a new blank ampState map, which we'll use to decode the
	// bytes into.
	ampState2 := make(AMPInvoiceState)

	// Decode from the raw stream into this blank mpa.
	tlvStream, err = tlv.NewStream(
		tlv.MakeDynamicRecord(
			invoiceAmpStateType, &ampState2, nil,
			ampStateEncoder, ampStateDecoder,
		),
	)
	require.Nil(t, err)

	err = tlvStream.Decode(&b)
	require.Nil(t, err)

	// The two states should match.
	require.Equal(t, ampState, ampState2)
}
