package blindedpath

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tlv"
)

// BuildBlindedMessagePathCfg defines the resources and configuration needed to
// build blinded onion message paths to this node. These paths are used for
// reply paths in invoice requests and for offer_paths in offers.
type BuildBlindedMessagePathCfg struct {
	// FindRoutes returns a set of routes to us that can be used for
	// blinded message path construction. These routes will consist of
	// real nodes advertising the onion messages feature bit.
	FindRoutes func() ([]*route.Route, error)

	// PathID is optional secret data to embed in the final hop's
	// encrypted data. For offer_paths this can correlate incoming
	// requests to a specific offer.
	PathID []byte
}

// BuildBlindedMessagePaths constructs a set of blinded onion message paths to
// this node using the provided configuration. Each path is suitable for use as
// a reply_path or offer_path in BOLT 12 messages.
func BuildBlindedMessagePaths(cfg *BuildBlindedMessagePathCfg) (
	[]*sphinx.BlindedPathInfo, error) {

	routes, err := cfg.FindRoutes()
	if err != nil {
		return nil, err
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("could not find any routes to self " +
			"for blinded message path construction")
	}

	paths := make([]*sphinx.BlindedPathInfo, 0, len(routes))
	for _, rt := range routes {
		path, err := buildBlindedMessagePath(rt, cfg.PathID)
		if err != nil {
			log.Errorf("Not using route (%s) as a blinded "+
				"message path: %v", rt, err)

			continue
		}

		paths = append(paths, path)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("could not build any blinded " +
			"message paths")
	}

	return paths, nil
}

// buildBlindedMessagePath converts a single route into a blinded onion message
// path. Each non-final hop's encrypted data contains only the next_node_id.
// The final hop's encrypted data contains an optional path_id.
func buildBlindedMessagePath(rt *route.Route,
	pathID []byte) (*sphinx.BlindedPathInfo, error) {

	// Build the list of hops. For a route with N hops, we have N+1 nodes
	// (intro node + N hop nodes). The sphinx HopInfo list has one entry
	// per node that will receive encrypted data.
	//
	// For a route: intro -> hop1 -> hop2 (us)
	//   HopInfo[0]: intro node, plaintext = next_node_id(hop1)
	//   HopInfo[1]: hop1, plaintext = next_node_id(hop2)
	//   HopInfo[2]: hop2 (us), plaintext = path_id (optional)
	//
	// For a self-only path (no hops):
	//   HopInfo[0]: us, plaintext = path_id (optional)
	numHops := len(rt.Hops)
	hops := make([]*sphinx.HopInfo, 0, numHops+1)

	// Parse the introduction node's public key.
	introPub, err := btcec.ParsePubKey(rt.SourcePubKey[:])
	if err != nil {
		return nil, fmt.Errorf("parse intro node pubkey: %w", err)
	}

	if numHops == 0 {
		// Self-only path: the introduction node is also the
		// destination.
		plainText, err := encodeFinalHopData(pathID)
		if err != nil {
			return nil, err
		}

		hops = append(hops, &sphinx.HopInfo{
			NodePub:   introPub,
			PlainText: plainText,
		})
	} else {
		// Non-final intro hop: forward to first hop node.
		firstHopPub, err := btcec.ParsePubKey(
			rt.Hops[0].PubKeyBytes[:],
		)
		if err != nil {
			return nil, fmt.Errorf("parse hop 0 pubkey: %w",
				err)
		}

		introData := record.NewNonFinalBlindedRouteDataOnionMessage(
			fn.NewLeft[
				*btcec.PublicKey, lnwire.ShortChannelID,
			](firstHopPub),
			nil, nil,
		)

		introPlainText, err := record.EncodeBlindedRouteData(
			introData,
		)
		if err != nil {
			return nil, fmt.Errorf("encode intro hop data: %w",
				err)
		}

		hops = append(hops, &sphinx.HopInfo{
			NodePub:   introPub,
			PlainText: introPlainText,
		})

		// Intermediate hops (if any) and the final hop.
		for i := 0; i < numHops; i++ {
			hopPub, err := btcec.ParsePubKey(
				rt.Hops[i].PubKeyBytes[:],
			)
			if err != nil {
				return nil, fmt.Errorf(
					"parse hop %d pubkey: %w", i, err,
				)
			}

			isFinal := i == numHops-1
			var plainText []byte

			if isFinal {
				plainText, err = encodeFinalHopData(
					pathID,
				)
				if err != nil {
					return nil, err
				}
			} else {
				nextPub, err := btcec.ParsePubKey(
					rt.Hops[i+1].PubKeyBytes[:],
				)
				if err != nil {
					return nil, fmt.Errorf(
						"parse hop %d pubkey: %w",
						i+1, err,
					)
				}

				data := record.NewNonFinalBlindedRouteDataOnionMessage( //nolint:ll
					fn.NewLeft[
						*btcec.PublicKey,
						lnwire.ShortChannelID,
					](nextPub),
					nil, nil,
				)

				plainText, err = record.EncodeBlindedRouteData( //nolint:ll
					data,
				)
				if err != nil {
					return nil, fmt.Errorf(
						"encode hop %d data: %w",
						i, err,
					)
				}
			}

			hops = append(hops, &sphinx.HopInfo{
				NodePub:   hopPub,
				PlainText: plainText,
			})
		}
	}

	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}

	blindedPath, err := sphinx.BuildBlindedPath(sessionKey, hops)
	if err != nil {
		return nil, fmt.Errorf("build blinded path: %w", err)
	}

	return blindedPath, nil
}

// encodeFinalHopData encodes the route data for the final hop (our node).
// If a pathID is provided, it is embedded in the encrypted data.
func encodeFinalHopData(pathID []byte) ([]byte, error) {
	data := &record.BlindedRouteData{}

	if len(pathID) > 0 {
		data.PathID = tlv.SomeRecordT(
			tlv.RecordT[tlv.TlvType6, []byte]{Val: pathID},
		)
	}

	buf, err := record.EncodeBlindedRouteData(data)
	if err != nil {
		return nil, fmt.Errorf("encode final hop data: %w", err)
	}

	return buf, nil
}
