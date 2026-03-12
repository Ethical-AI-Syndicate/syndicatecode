package controlplane

import (
	"testing"
	"time"
)

func TestStreamBus_PublishSubscribe_Bead_l3d_X_1(t *testing.T) {
	bus := newStreamBus()
	ch, unsub := bus.subscribe("sess-1")
	defer unsub()

	bus.publish("sess-1", streamMessage{Type: "text_delta", Data: "hello"})

	select {
	case msg := <-ch:
		if msg.Data != "hello" {
			t.Errorf("got %q, want %q", msg.Data, "hello")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for message")
	}
}

func TestStreamBus_NoLeakAcrossSessions_Bead_l3d_X_1(t *testing.T) {
	bus := newStreamBus()
	ch, unsub := bus.subscribe("sess-1")
	defer unsub()

	bus.publish("sess-2", streamMessage{Type: "text_delta", Data: "other"})

	select {
	case msg := <-ch:
		t.Fatalf("received unexpected message for wrong session: %v", msg)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestStreamBus_Unsubscribe_Bead_l3d_X_1(t *testing.T) {
	bus := newStreamBus()
	_, unsub := bus.subscribe("sess-1")
	unsub()
	bus.publish("sess-1", streamMessage{Type: "text_delta", Data: "after-unsub"})
}
