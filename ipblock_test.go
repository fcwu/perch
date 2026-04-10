package main

import (
	"testing"
)

func TestIPBlockListParsing(t *testing.T) {
	bl := newIPBlockList([]string{"192.168.1.1", "10.0.0.0/8"})

	if !bl.isBlocked("192.168.1.1") {
		t.Error("192.168.1.1 should be blocked")
	}
	if !bl.isBlocked("10.5.5.5") {
		t.Error("10.5.5.5 should be blocked (subnet)")
	}
	if bl.isBlocked("8.8.8.8") {
		t.Error("8.8.8.8 should not be blocked")
	}
}
