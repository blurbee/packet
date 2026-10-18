package kiss

import "time"

// devices holds the device definitions: the built-in ones below, plus any
// loaded from definition files (see fetch).  Everything particular to a device
// is here, not in the transport.
var devices = map[string]*Device{
	// The Kenwood TH-D75's built-in TNC supports only KISS mode for packet
	// connections.  It is reached over USB or Bluetooth, whichever is
	// selected in the radio's KISS interface menu setting.
	//
	// Its TNC must be in full-duplex mode (ignoring its carrier detect).
	// In half duplex, it often held our frames for seconds, or until our
	// next write, though the channel was quiet (its BUSY light off): the
	// BBS got our first copy together with our retransmission, and
	// rejected the duplicate, or nothing at all (the radio stuck showing
	// STA).  Idle, it transmitted within a second either way.  In full
	// duplex, a whole session with W3XSC went without a single retry.
	//
	// Over USB, it ignores data from the computer while DTR is low, which
	// is how macOS leaves it.  Its TNC can be reset from the computer with
	// Kenwood's TN command (see kenwoodReset), which it sometimes needs
	// (showing STA) after a session that didn't end cleanly.
	//
	// The link settings suit W3XSC, which answers in about 3 seconds once
	// the radio transmits promptly, and sometimes sends two or three
	// frames up to a second apart (hence the 2-second turnaround): two
	// frames per turn (each turn costs the BBS's answer time plus the
	// turnaround, about 5 seconds, so sending two halves the turns a long
	// message takes), and T1 starting at 10 seconds, then adapting to the
	// measured answer time (but no lower than 6 seconds), backing off for
	// each retry; 6 retries cover over 2 minutes of silence from the BBS.
	"Kenwood TH-D75": {
		Name:         "Kenwood TH-D75",
		PacLen:       128,
		MaxFrame:     2,
		Frack:        6 * time.Second,
		InitialFrack: 10 * time.Second,
		Retry:        6,
		RespTime:     500 * time.Millisecond,
		Turnaround:   2 * time.Second,
		TXDelay:      400 * time.Millisecond,
		Persist:      63,
		SlotTime:     100 * time.Millisecond,
		FullDuplex:   true,
		AssertDTR:    true,
		AssertRTS:    true,
		Reset:        resetKenwood,
		// Long enough for a frame of up to PacLen bytes, with TXDelay,
		// to get on the air at 1200 baud.
		ResetSettle: 3 * time.Second,
	},
	// A generic KISS TNC: standard AX.25 settings, and nothing done to the
	// device beyond the standard KISS parameters.
	"Generic KISS TNC": {
		Name:     "Generic KISS TNC",
		PacLen:   128,
		MaxFrame: 2,
		Frack:    6 * time.Second,
		Retry:    8,
		// Without measurements, wait twice Frack before resending.
		InitialFrack: 12 * time.Second,
		RespTime:     500 * time.Millisecond,
		Turnaround:   2 * time.Second,
		TXDelay:      400 * time.Millisecond,
		Persist:      63,
		SlotTime:     100 * time.Millisecond,
	},
}
