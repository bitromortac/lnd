package keychain

import (
	"errors"

	"github.com/btcsuite/btcd/btcec/v2"
)

var (
	// ErrInvalidMulTweak is returned when a multiplicative tweak is not a
	// 32-byte scalar in the range 1 to n-1.
	ErrInvalidMulTweak = errors.New("multiplicative tweak is not a " +
		"valid non-zero scalar")

	// ErrZeroTweakedKey is returned when a tweak gives the zero key.
	ErrZeroTweakedKey = errors.New("tweaked key is zero")
)

// parseMulTweak parses a multiplicative tweak as a non-zero scalar.
func parseMulTweak(tweak []byte) (*btcec.ModNScalar, error) {
	if len(tweak) != 32 {
		return nil, ErrInvalidMulTweak
	}

	var scalar btcec.ModNScalar
	if overflow := scalar.SetByteSlice(tweak); overflow {
		return nil, ErrInvalidMulTweak
	}
	if scalar.IsZero() {
		return nil, ErrInvalidMulTweak
	}

	return &scalar, nil
}

// MulTweakPrivKey multiplies privKey by the 32-byte scalar tweak modulo the
// curve order. Route blinding derives a blinded node key in this way.
func MulTweakPrivKey(privKey *btcec.PrivateKey,
	tweak []byte) (*btcec.PrivateKey, error) {

	scalar, err := parseMulTweak(tweak)
	if err != nil {
		return nil, err
	}

	var tweaked btcec.ModNScalar
	tweaked.Set(&privKey.Key)
	tweaked.Mul(scalar)
	if tweaked.IsZero() {
		return nil, ErrZeroTweakedKey
	}

	return btcec.PrivKeyFromScalar(&tweaked), nil
}

// MulTweakPubKey multiplies pubKey by the 32-byte scalar tweak. It is the
// public key of MulTweakPrivKey.
func MulTweakPubKey(pubKey *btcec.PublicKey,
	tweak []byte) (*btcec.PublicKey, error) {

	scalar, err := parseMulTweak(tweak)
	if err != nil {
		return nil, err
	}

	var point, tweaked btcec.JacobianPoint
	pubKey.AsJacobian(&point)
	btcec.ScalarMultNonConst(scalar, &point, &tweaked)
	if (tweaked.X.IsZero() && tweaked.Y.IsZero()) || tweaked.Z.IsZero() {
		return nil, ErrZeroTweakedKey
	}
	tweaked.ToAffine()

	return btcec.NewPublicKey(&tweaked.X, &tweaked.Y), nil
}
