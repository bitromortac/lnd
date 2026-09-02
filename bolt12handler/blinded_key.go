package bolt12handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

// routeBlindingHMACKey is the HMAC key that BOLT 4 route blinding uses to turn
// a hop's shared secret into its blinding factor.
const routeBlindingHMACKey = "blinded_node_id"

// blindedKeyRef names the key behind the blinded_node_id that a route-blinding
// path creator assigned to this node for pathKey, the ephemeral key the
// message arrived under.
//
// The creator published our identity key multiplied by a factor it derived
// from the shared secret with pathKey. That secret is reproducible with our
// identity key alone, so the same factor as a multiplicative tweak of our key
// gives the key that signs for the published blinded identity. A node that
// only relays the path cannot do this, which is what makes the
// blinded_node_id binding meaningful to the payer.
func blindedKeyRef(signer Signer, pathKey *btcec.PublicKey) (KeyRef, error) {
	if pathKey == nil {
		return KeyRef{}, fmt.Errorf("nil path key")
	}

	sharedSecret, err := signer.ECDH(pathKey)
	if err != nil {
		return KeyRef{}, fmt.Errorf("path key ECDH: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(routeBlindingHMACKey))
	mac.Write(sharedSecret[:])

	return KeyRef{MulTweak: mac.Sum(nil)}, nil
}

// blindedNodeID returns the blinded_node_id of this node for pathKey.
func blindedNodeID(signer Signer,
	pathKey *btcec.PublicKey) (*btcec.PublicKey, error) {

	key, err := blindedKeyRef(signer, pathKey)
	if err != nil {
		return nil, err
	}

	return signer.PubKey(key)
}
