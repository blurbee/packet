package kiss

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// A Device describes a KISS TNC: the AX.25 link parameters to use with it, and
// anything about the device itself that the transport must handle.  Devices
// are defined as data: the built-in definitions (see builtin.go) plus any
// definition files in the KISS device directory (see KISSDir and DeviceFile).
// The transport has no knowledge of particular devices beyond what their
// definitions say.
type Device struct {
	// Name is the name of the device.
	Name string
	// PacLen is the maximum number of data bytes in each packet.
	PacLen int
	// MaxFrame is the maximum number of unacknowledged packets that can be
	// outstanding at once.
	MaxFrame int
	// Frack is how long to wait for an acknowledgment from the BBS before
	// polling it, not counting the time it takes to transmit the
	// unacknowledged packets.  It is also the shortest the adaptive retry
	// timer ever gets.
	Frack time.Duration
	// InitialFrack is the Frack to use until the BBS's answer time has been
	// measured.  It is longer than Frack, so that a slow BBS doesn't get a
	// retransmission while its answer to the first copy is still on the
	// way; as the link measures the BBS's actual answer time, the wait
	// adapts to it (but never drops below Frack).  Zero means Frack.
	InitialFrack time.Duration
	// Retry is the number of times to poll the BBS without a response
	// before deciding the link has failed.
	Retry int
	// RespTime is how long the channel must be quiet after a packet from
	// the BBS before we send a plain acknowledgment, so that the BBS can
	// finish sending a burst of packets first.  Acknowledgments go sooner
	// when the BBS polls for one or when we have data to send, which
	// carries it.
	RespTime time.Duration
	// Turnaround is the minimum time after a packet from the BBS before we
	// transmit anything, giving the BBS's radio time to switch from
	// transmitting back to receiving, and keeping us from transmitting
	// into a burst of packets the BBS sends with short gaps between them.
	// (The TNC's carrier detect can't tell when the BBS is ready to hear
	// us: it sees only that the channel is clear.)  With FullDuplex, it is
	// all that keeps us off the BBS's transmissions.  This is the
	// equivalent of a TNC's DWAIT or RESPTIME setting.
	Turnaround time.Duration
	// TXDelay, Persist and SlotTime are sent to the TNC at connect time
	// (as KISS commands), to control how it decides when to transmit.  The
	// TNC keys up only when its carrier detect shows the channel is clear,
	// then (each slot time) with probability (Persist+1)/256, and then
	// sends TXDelay worth of preamble before the data, which also gives a
	// BBS that just stopped transmitting time to start receiving.  Zero
	// values are not sent, leaving the TNC's own settings in effect.
	TXDelay  time.Duration
	Persist  int
	SlotTime time.Duration
	// FullDuplex makes the TNC transmit as soon as it has a frame, without
	// waiting for its carrier detect to show the channel clear (KISS
	// command 05).  We still wait out Turnaround after the BBS's frames.
	// It is restored to half duplex when the session ends.  A full-duplex
	// TNC holds no frames, so a session the BBS ends while it has data of
	// ours unacknowledged needs no reset.
	FullDuplex bool
	// AssertDTR and AssertRTS raise the DTR and RTS lines of the serial
	// port after opening it.  Some TNCs ignore data from the computer while
	// DTR is low (which is how some systems leave it); others reset when
	// it changes, so it is left alone unless the device calls for it.
	AssertDTR bool
	AssertRTS bool
	// Reset names the method for resetting the TNC to a clean state,
	// discarding any frames it still holds (see resetMethods), or is empty
	// if the TNC can't be reset from the computer.  A TNC with a reset
	// method is reset after a session that didn't end cleanly, and before a
	// session if the previous one never finished (the program was killed,
	// say).
	Reset string
	// ResetSettle is how long after our last transmission to wait before
	// resetting the TNC, so that a final frame (such as our UA to the BBS's
	// DISC) has time to get on the air rather than being discarded.
	ResetSettle time.Duration
}

// DeviceFile is the format of a KISS device definition file: a JSON object
// with these members (names are not case-sensitive).  Times are in
// milliseconds.  Zero or missing values for TXDelayMs, Persist and SlotTimeMs
// leave the TNC's own settings in effect; for InitialFrackMs, it means FrackMs.
type DeviceFile struct {
	Name           string `json:"name"`
	PacLen         int    `json:"pacLen"`
	MaxFrame       int    `json:"maxFrame"`
	FrackMs        int    `json:"frackMs"`
	InitialFrackMs int    `json:"initialFrackMs,omitzero"`
	Retry          int    `json:"retry"`
	RespTimeMs     int    `json:"respTimeMs"`
	TurnaroundMs   int    `json:"turnaroundMs"`
	TXDelayMs      int    `json:"txDelayMs,omitzero"`
	Persist        int    `json:"persist,omitzero"`
	SlotTimeMs     int    `json:"slotTimeMs,omitzero"`
	FullDuplex     bool   `json:"fullDuplex,omitzero"`
	AssertDTR      bool   `json:"assertDTR,omitzero"`
	AssertRTS      bool   `json:"assertRTS,omitzero"`
	Reset          string `json:"reset,omitzero"`
	ResetSettleMs  int    `json:"resetSettleMs,omitzero"`
}

// File returns the definition file form of the device.
func (d *Device) File() DeviceFile {
	ms := func(t time.Duration) int { return int(t / time.Millisecond) }
	return DeviceFile{
		Name: d.Name, PacLen: d.PacLen, MaxFrame: d.MaxFrame, FrackMs: ms(d.Frack),
		InitialFrackMs: ms(d.InitialFrack), Retry: d.Retry, RespTimeMs: ms(d.RespTime),
		TurnaroundMs: ms(d.Turnaround), TXDelayMs: ms(d.TXDelay), Persist: d.Persist,
		SlotTimeMs: ms(d.SlotTime), FullDuplex: d.FullDuplex, AssertDTR: d.AssertDTR,
		AssertRTS: d.AssertRTS, Reset: d.Reset, ResetSettleMs: ms(d.ResetSettle),
	}
}

// Device returns the device defined by the definition file.
func (f DeviceFile) Device() *Device {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	return &Device{
		Name: f.Name, PacLen: f.PacLen, MaxFrame: f.MaxFrame, Frack: ms(f.FrackMs),
		InitialFrack: ms(f.InitialFrackMs), Retry: f.Retry, RespTime: ms(f.RespTimeMs),
		Turnaround: ms(f.TurnaroundMs), TXDelay: ms(f.TXDelayMs), Persist: f.Persist,
		SlotTime: ms(f.SlotTimeMs), FullDuplex: f.FullDuplex, AssertDTR: f.AssertDTR,
		AssertRTS: f.AssertRTS, Reset: f.Reset, ResetSettle: ms(f.ResetSettleMs),
	}
}

// Limits on device parameters, enforced by Validate and validated.
const (
	// maxMaxFrame is the largest usable MaxFrame: with modulo 8 sequence
	// numbers, a window of 8 or more would make N(R) ambiguous.
	maxMaxFrame = 7
	// maxPacLen is the largest information field AX.25 allows.
	maxPacLen = 256
	// minFrack is the shortest Frack we allow.  It is far too short for
	// real use at 1200 baud; it just keeps a missing or nonsensical value
	// from making the link spin.
	minFrack = 100 * time.Millisecond
	// maxKISSTime is the longest TXDelay or SlotTime a KISS command can
	// set (255 units of 10ms).
	maxKISSTime = 2550 * time.Millisecond
)

// Validate returns an error if the device definition is not usable.
func (d *Device) Validate() error {
	if d == nil {
		return errors.New("no device definition")
	}
	switch {
	case strings.TrimSpace(d.Name) == "":
		return errors.New("Name is missing or blank")
	case d.PacLen < 1 || d.PacLen > maxPacLen:
		return fmt.Errorf("PacLen must be between 1 and %d", maxPacLen)
	case d.MaxFrame < 1 || d.MaxFrame > maxMaxFrame:
		return fmt.Errorf("MaxFrame must be between 1 and %d", maxMaxFrame)
	case d.Frack < minFrack:
		return fmt.Errorf("Frack must be at least %v", minFrack)
	case d.InitialFrack < 0:
		return errors.New("InitialFrack is negative")
	case d.Retry < 1:
		return errors.New("Retry must be at least 1")
	case d.RespTime < 0, d.Turnaround < 0, d.ResetSettle < 0:
		return errors.New("a time is negative")
	case d.TXDelay < 0 || d.TXDelay > maxKISSTime, d.SlotTime < 0 || d.SlotTime > maxKISSTime:
		return fmt.Errorf("TXDelay and SlotTime must be between 0 and %v", maxKISSTime)
	case d.Persist < 0 || d.Persist > 255:
		return errors.New("Persist must be between 0 and 255")
	}
	if _, ok := resetMethods[d.Reset]; d.Reset != "" && !ok {
		return fmt.Errorf("Reset %q is not a known reset method (known: %s)", d.Reset, strings.Join(slices.Sorted(maps.Keys(resetMethods)), ", "))
	}
	return nil
}

// validated returns a copy of d with its link parameters forced into their
// valid ranges.  (Definitions are validated when loaded; this guards against
// devices built some other way, e.g., in tests.)
func (d *Device) validated() *Device {
	v := *d
	v.MaxFrame = min(max(v.MaxFrame, 1), maxMaxFrame)
	v.PacLen = min(max(v.PacLen, 1), maxPacLen)
	v.Frack = max(v.Frack, minFrack)
	v.InitialFrack = min(max(v.InitialFrack, v.Frack), max(maxFrack, v.Frack))
	v.Retry = max(v.Retry, 1)
	v.RespTime = max(v.RespTime, 0)
	v.Turnaround = max(v.Turnaround, 0)
	v.ResetSettle = max(v.ResetSettle, 0)
	return &v
}

var fetchOnce sync.Once

// fetch adds the device definitions in the KISS device directory to the
// built-in ones, replacing any of the same name.  Files that can't be read or
// aren't valid are skipped, with a warning in the log.
func fetch() {
	dir := KISSDir()
	if dir == "" {
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("os.ReadDir", "d", dir, "err", err)
		}
		return
	}
	for _, ent := range ents {
		if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".json") {
			fetchOne(filepath.Join(dir, ent.Name()))
		}
	}
}

func fetchOne(path string) {
	var f DeviceFile
	if by, err := os.ReadFile(path); err != nil {
		slog.Warn("os.ReadFile", "f", path, "err", err)
	} else if err = json.Unmarshal(by, &f, json.MatchCaseInsensitiveNames(true), json.RejectUnknownMembers(true)); err != nil {
		slog.Warn("json.Unmarshal", "f", path, "err", err)
	} else if d := f.Device(); d.Validate() != nil {
		slog.Warn("invalid KISS device definition", "f", path, "err", d.Validate())
	} else {
		devices[d.Name] = d
	}
}

// AllDevices returns an alphabetical list of the names of all supported KISS
// devices.
func AllDevices() []string {
	fetchOnce.Do(fetch)
	return slices.Sorted(maps.Keys(devices))
}

// GetDevice returns the definition of the KISS device with the specified name,
// or nil if there is none.
func GetDevice(name string) *Device {
	fetchOnce.Do(fetch)
	return devices[name]
}
