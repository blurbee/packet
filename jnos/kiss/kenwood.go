package kiss

import (
	"bytes"
	"fmt"
	"regexp"
	"time"
)

// This file holds what is particular to Kenwood radios' built-in TNCs (such as
// the TH-D75's): resetting the TNC with Kenwood's CAT commands.  Device
// definitions select it with Reset: "kenwood".

// resetKenwood is the name of the Kenwood reset method (see kenwoodReset), for
// device definitions' Reset field.
const resetKenwood = "kenwood"

// Timing of Kenwood's CAT command interface.  They are variables so that tests
// can shorten them.
var (
	// kenwoodExitWait is how long to wait after the KISS exit command for
	// the radio to leave KISS mode.
	kenwoodExitWait = 300 * time.Millisecond
	// kenwoodReplyWait is how long to wait for the radio to answer a
	// command.
	kenwoodReplyWait = 1500 * time.Millisecond
)

// tnRE matches the radio's answer to the TN command: the TNC mode (0 off, 1
// APRS, 2 KISS) and a second setting, which we preserve.
var tnRE = regexp.MustCompile(`^TN (\d),(\d)$`)

// kenwoodReset is the reset method (see resetMethod) for a Kenwood radio's
// built-in TNC, which can be controlled with Kenwood's CAT commands on the
// same port: it leaves KISS mode, which discards any frames the TNC still
// holds, and then re-enters KISS mode with the TN command, keeping the second
// TN setting that the radio reports.  It sends nothing that transmits.
func kenwoodReset(port resetPort) (did string, err error) {
	var reentered string

	if reentered, err = kenwoodReenterKISS(port); err != nil {
		return "", fmt.Errorf("%w; reset it on the radio: turn the TNC off and back to KISS", err)
	}
	return "left KISS mode, discarding anything it still held, and re-entered it with " + reentered, nil
}

// kenwoodReenterKISS does the work of kenwoodReset, returning the command used
// to re-enter KISS mode.
func kenwoodReenterKISS(port resetPort) (reentered string, err error) {
	// The KISS "return" command: leave KISS mode.
	if _, err = port.Write([]byte{fend, 0xFF, fend}); err != nil {
		return "", fmt.Errorf("send KISS exit: %w", err)
	}
	time.Sleep(kenwoodExitWait)
	// If the radio wasn't in KISS mode, it took the KISS exit bytes as the
	// start of a command, and would answer our next one with "?".  A bare
	// carriage return ends that garbage command; its answer is drained.
	if _, err = port.Write([]byte{'\r'}); err != nil {
		return "", fmt.Errorf("send CR: %w", err)
	}
	time.Sleep(kenwoodExitWait)
	drain(port)
	// Ask for the TNC setting.  Out of KISS mode, the radio answers "TN
	// 0,n"; if it doesn't answer, it isn't in the state we expect, so stop
	// rather than guess.  (A "?" gets one more try.)
	reply, err := catCommand(port, "TN")
	if err == nil && reply == "?" {
		drain(port)
		reply, err = catCommand(port, "TN")
	}
	if err != nil {
		return "", err
	}
	match := tnRE.FindStringSubmatch(reply)
	if match == nil {
		return "", fmt.Errorf("unexpected answer to TN: %q", reply)
	}
	reentered = "TN 2," + match[2]
	if reply, err = catCommand(port, reentered); err != nil {
		return "", err
	}
	if reply != reentered {
		return "", fmt.Errorf("unexpected answer to %s: %q", reentered, reply)
	}
	return reentered, nil
}

// catCommand sends a command to the radio and returns its one-line answer.
func catCommand(port resetPort, cmd string) (reply string, err error) {
	if _, err = port.Write([]byte(cmd + "\r")); err != nil {
		return "", fmt.Errorf("send %s: %w", cmd, err)
	}
	var (
		got      []byte
		buf      = make([]byte, 64)
		deadline = time.Now().Add(kenwoodReplyWait)
	)
	for {
		if idx := bytes.IndexByte(got, '\r'); idx >= 0 {
			return string(bytes.TrimSpace(got[:idx])), nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", fmt.Errorf("%s: %w", cmd, errNoAnswer)
		}
		if err = port.SetReadTimeout(remaining); err != nil {
			return "", err
		}
		count, err := port.Read(buf)
		got = append(got, buf[:count]...)
		if err != nil {
			return "", fmt.Errorf("%s: %w", cmd, err)
		}
	}
}
