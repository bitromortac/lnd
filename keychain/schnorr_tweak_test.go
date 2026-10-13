package keychain

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/stretchr/testify/require"
)

// TestMulTweak verifies that the private and the public multiplicative tweak
// agree, so a signature with the tweaked private key verifies against the
// tweaked public key.
func TestMulTweak(t *testing.T) {
	t.Parallel()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	var tweak [32]byte
	tweak[31] = 7

	tweakedPriv, err := MulTweakPrivKey(privKey, tweak[:])
	require.NoError(t, err)

	tweakedPub, err := MulTweakPubKey(privKey.PubKey(), tweak[:])
	require.NoError(t, err)
	require.True(t, tweakedPriv.PubKey().IsEqual(tweakedPub))
	require.False(t, tweakedPub.IsEqual(privKey.PubKey()))

	digest := chainhash.TaggedHash([]byte("test"), []byte("msg"))
	sig, err := schnorr.Sign(tweakedPriv, digest[:])
	require.NoError(t, err)
	require.True(t, sig.Verify(digest[:], tweakedPub))
}

// TestMulTweakInvalid verifies that a tweak that is not a non-zero scalar
// is rejected.
func TestMulTweakInvalid(t *testing.T) {
	t.Parallel()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	tests := []struct {
		name  string
		tweak []byte
	}{
		{
			name:  "short",
			tweak: []byte{1},
		},
		{
			name:  "zero",
			tweak: make([]byte, 32),
		},
		{
			name: "curve order",
			tweak: func() []byte {
				b := btcec.S256().N.Bytes()
				return b
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := MulTweakPrivKey(privKey, tc.tweak)
			require.ErrorIs(t, err, ErrInvalidMulTweak)

			_, err = MulTweakPubKey(privKey.PubKey(), tc.tweak)
			require.ErrorIs(t, err, ErrInvalidMulTweak)
		})
	}
}
