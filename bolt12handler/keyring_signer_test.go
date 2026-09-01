package bolt12handler

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/lightningnetwork/lnd/bolt12"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lntest/mock"
	"github.com/stretchr/testify/require"
)

// testKeyRingSigner returns a KeyRingSigner over a mock key ring that holds
// nodeKey.
func testKeyRingSigner(nodeKey *btcec.PrivateKey) *KeyRingSigner {
	return NewKeyRingSigner(
		&ecdhKeyRing{&mock.SecretKeyRing{RootKey: nodeKey}},
		keychain.KeyLocator{Family: keychain.KeyFamilyNodeKey},
		nodeKey.PubKey(),
	)
}

// ecdhKeyRing is a mock key ring with a real ECDH of its root key. The ECDH of
// mock.SecretKeyRing returns zeros.
type ecdhKeyRing struct {
	*mock.SecretKeyRing
}

// ECDH returns the shared secret of the root key and pubKey.
func (k *ecdhKeyRing) ECDH(_ keychain.KeyDescriptor,
	pubKey *btcec.PublicKey) ([32]byte, error) {

	return (&keychain.PrivKeyECDH{PrivKey: k.RootKey}).ECDH(pubKey)
}

// TestKeyRingSignerSignSchnorr verifies that KeyRingSigner signs the tagged
// hash of the message with the node key and with each tweaked key, and that
// PubKey names the key that verifies the signature.
func TestKeyRingSignerSignSchnorr(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	signer := testKeyRingSigner(nodeKey)

	var mulTweak, addTweak [32]byte
	mulTweak[31] = 0x05
	addTweak[0] = 0xAB

	tests := []struct {
		name string
		key  KeyRef
	}{
		{
			name: "node key",
		},
		{
			name: "multiplicative tweak",
			key:  KeyRef{MulTweak: mulTweak[:]},
		},
		{
			name: "additive tweak",
			key:  KeyRef{AddTweak: addTweak[:]},
		},
		{
			name: "both tweaks",
			key: KeyRef{
				MulTweak: mulTweak[:],
				AddTweak: addTweak[:],
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := []byte("root")
			sigBytes, err := signer.SignSchnorr(
				tc.key, bolt12.InvoiceSignatureTag, msg,
			)
			require.NoError(t, err)

			sig, err := schnorr.ParseSignature(sigBytes[:])
			require.NoError(t, err)

			pub, err := signer.PubKey(tc.key)
			require.NoError(t, err)

			// The private side of the tweaks gives the same key.
			priv, err := tweakPrivKey(nodeKey, tc.key)
			require.NoError(t, err)
			require.True(t, priv.PubKey().IsEqual(pub))

			digest := chainhash.TaggedHash(
				[]byte(bolt12.InvoiceSignatureTag), msg,
			)
			require.True(t, sig.Verify(digest[:], pub))
		})
	}
}

// TestKeyRingSignerECDH verifies that the signer derives the shared secret
// with the node key.
func TestKeyRingSignerECDH(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)
	otherKey := testKey2(t)

	secret, err := testKeyRingSigner(nodeKey).ECDH(otherKey.PubKey())
	require.NoError(t, err)

	want, err := (&keychain.PrivKeyECDH{PrivKey: nodeKey}).ECDH(
		otherKey.PubKey(),
	)
	require.NoError(t, err)
	require.Equal(t, want, secret)
}

// TestKeyRingSignerSignEnvelope verifies that the key ring signer signs an
// envelope under the node key, so VerifyEnvelope accepts it.
func TestKeyRingSignerSignEnvelope(t *testing.T) {
	t.Parallel()

	nodeKey := testKey(t)

	var offerHash [32]byte
	offerHash[0] = 0xCD
	data := &InvoiceEnvelopeData{Amount: 1000}

	signed, err := SignEnvelope(testKeyRingSigner(nodeKey), offerHash, data)
	require.NoError(t, err)

	got, err := VerifyEnvelope(nodeKey.PubKey(), signed)
	require.NoError(t, err)
	require.Equal(t, data.Amount, got.Amount)

	// A signature over another offer hash must fail.
	signed.OfferHash[0] = 0xCE
	_, err = VerifyEnvelope(nodeKey.PubKey(), signed)
	require.Error(t, err)
}
