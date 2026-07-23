package htlcswitch

import (
	"testing"

	"github.com/lightningnetwork/lnd/htlcswitch/hop"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// TestGetEventType asserts how getEventType classifies an htlcPacket as a send,
// receive or forward event.
func TestGetEventType(t *testing.T) {
	t.Parallel()

	var nodeID [33]byte
	nodeID[0] = 0x02

	tests := []struct {
		name string
		pkt  *htlcPacket
		want HtlcEventType
	}{
		{
			name: "send",
			pkt:  &htlcPacket{incomingChanID: hop.Source},
			want: HtlcEventTypeSend,
		},
		{
			name: "receive at exit hop",
			pkt: &htlcPacket{
				incomingChanID: lnwire.NewShortChanIDFromInt(1),
				outgoingChanID: hop.Exit,
			},
			want: HtlcEventTypeReceive,
		},
		{
			name: "forward by channel ID",
			pkt: &htlcPacket{
				incomingChanID: lnwire.NewShortChanIDFromInt(1),
				outgoingChanID: lnwire.NewShortChanIDFromInt(2),
			},
			want: HtlcEventTypeForward,
		},
		{
			// A node-ID forward that failed before channel
			// selection has outgoingChanID == hop.Exit but a Right
			// (pubkey) next hop, so it must classify as a forward.
			name: "forward by node ID before selection",
			pkt: &htlcPacket{
				incomingChanID: lnwire.NewShortChanIDFromInt(1),
				outgoingChanID: hop.Exit,
				outgoingHop:    hop.NewNodeNextHop(nodeID),
			},
			want: HtlcEventTypeForward,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, getEventType(tc.pkt))
		})
	}
}
