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
