package bolt12handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/keychain"
)

// routeBlindingHMACKey is the HMAC key that BOLT 4 route blinding uses to turn
// a hop's shared secret into its blinding factor.
const routeBlindingHMACKey = "blinded_node_id"

// blindPrivKey derives the private key behind the blinded_node_id that a
// route-blinding path creator assigned to this node for pathKey, the ephemeral
// key the message arrived under.
//
// The creator published our identity key multiplied by a factor it derived
// from the shared secret with pathKey. That secret is reproducible with our
// identity key alone, so applying the same factor to our private key yields
// the key that signs for the published blinded identity. A node that only
// relays the path cannot do this, which is what makes the blinded_node_id
// binding meaningful to the payer.
func blindPrivKey(priv *btcec.PrivateKey,
	pathKey *btcec.PublicKey) (*btcec.PrivateKey, error) {

	if priv == nil {
		return nil, fmt.Errorf("nil identity key")
	}

	if pathKey == nil {
		return nil, fmt.Errorf("nil path key")
	}

	ecdh := &keychain.PrivKeyECDH{PrivKey: priv}
	sharedSecret, err := ecdh.ECDH(pathKey)
	if err != nil {
		return nil, fmt.Errorf("path key ECDH: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(routeBlindingHMACKey))
	mac.Write(sharedSecret[:])

	var factor btcec.ModNScalar
	if overflow := factor.SetByteSlice(mac.Sum(nil)); overflow {
		return nil, fmt.Errorf("blinding factor overflows the curve " +
			"order")
	}

	if factor.IsZero() {
		return nil, fmt.Errorf("blinding factor is zero")
	}

	var blinded btcec.ModNScalar
	blinded.Mul2(&priv.Key, &factor)

	return btcec.PrivKeyFromScalar(&blinded), nil
}
