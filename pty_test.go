package main

import (
	"testing"
	"time"
)

func TestPTYManagerSubscribeAndBroadcast(t *testing.T) {
	pm := newPTYManager()
	ch, unsub := pm.subscribe()
	defer unsub()

	go pm.broadcast([]byte("hello"))

	select {
	case data := <-ch:
		if string(data) != "hello" {
			t.Fatalf("expected 'hello', got %q", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestPTYManagerUnsubscribeStopsDelivery(t *testing.T) {
	pm := newPTYManager()
	ch, unsub := pm.subscribe()
	unsub()

	go pm.broadcast([]byte("ignored"))

	select {
	case data := <-ch:
		t.Fatalf("should not receive after unsub, got %q", data)
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

func TestPTYManagerFramebufReplay(t *testing.T) {
	pm := newPTYManager()

	// simulate PTY output before any subscriber
	pm.broadcast([]byte("first"))
	pm.broadcast([]byte("second"))

	// new subscriber should receive framebuffer snapshot
	ch, unsub := pm.subscribe()
	defer unsub()

	select {
	case data := <-ch:
		if string(data) != "firstsecond" {
			t.Fatalf("expected framebuf replay 'firstsecond', got %q", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for framebuf replay")
	}
}

func TestPTYManagerFramebufCap(t *testing.T) {
	pm := newPTYManager()

	// fill past the 1MB cap
	big := make([]byte, maxFramebuf)
	pm.broadcast(big)
	pm.broadcast([]byte("tail"))

	if len(pm.framebuf) > maxFramebuf {
		t.Fatalf("framebuf exceeded cap: %d bytes", len(pm.framebuf))
	}
	// tail bytes must be present
	tail := pm.framebuf[len(pm.framebuf)-4:]
	if string(tail) != "tail" {
		t.Fatalf("expected tail to be at end of framebuf, got %q", tail)
	}
}

func TestPTYManagerFramebufClearedOnRestart(t *testing.T) {
	pm := newPTYManager()
	pm.broadcast([]byte("old data"))

	// simulate restart: clear framebuf
	pm.mu.Lock()
	pm.framebuf = nil
	pm.mu.Unlock()

	ch, unsub := pm.subscribe()
	defer unsub()

	// should receive nothing from framebuf
	select {
	case data := <-ch:
		t.Fatalf("expected no replay after restart, got %q", data)
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}
