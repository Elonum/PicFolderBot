package telegram

import (
	"errors"
	"net"
	"testing"
)

type tempNetErr struct{}

func (tempNetErr) Error() string   { return "temporary" }
func (tempNetErr) Timeout() bool   { return true }
func (tempNetErr) Temporary() bool { return true }

func TestIsTransientTelegramError(t *testing.T) {
	if !isTransientTelegramError(tempNetErr{}) {
		t.Fatalf("expected transient net error")
	}
	if !isTransientTelegramError(errors.New("unexpected EOF")) {
		t.Fatalf("expected transient EOF")
	}
	var netErr net.Error = tempNetErr{}
	if !isTransientTelegramError(netErr) {
		t.Fatalf("expected transient net interface")
	}
}
