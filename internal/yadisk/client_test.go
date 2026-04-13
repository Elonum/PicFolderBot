package yadisk

import (
	"errors"
	"net"
	"testing"
)

type tempErr struct{}

func (tempErr) Error() string   { return "timeout" }
func (tempErr) Timeout() bool   { return true }
func (tempErr) Temporary() bool { return true }

func TestIsTransientNetErr(t *testing.T) {
	if !isTransientNetErr(tempErr{}) {
		t.Fatalf("expected transient net error")
	}
	if !isTransientNetErr(errors.New("unexpected EOF")) {
		t.Fatalf("expected transient EOF")
	}
	var n net.Error = tempErr{}
	if !isTransientNetErr(n) {
		t.Fatalf("expected transient net interface")
	}
}
