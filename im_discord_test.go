package main

import (
	"regexp"
	"testing"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestChannelSessionIDFormat(t *testing.T) {
	id := channelSessionID("123456789")
	if !uuidRE.MatchString(id) {
		t.Fatalf("not a UUID: %q", id)
	}
}

func TestChannelSessionIDDeterministic(t *testing.T) {
	a := channelSessionID("abc")
	b := channelSessionID("abc")
	if a != b {
		t.Fatalf("not deterministic: %q vs %q", a, b)
	}
}

func TestChannelSessionIDUnique(t *testing.T) {
	if channelSessionID("111") == channelSessionID("222") {
		t.Fatal("different channels got the same UUID")
	}
}
