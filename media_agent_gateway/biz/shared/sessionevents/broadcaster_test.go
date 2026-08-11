package sessionevents

import "testing"

func TestPublishFansOutAndCancelIsIsolated(t *testing.T) {
	hub := New(2)
	one := hub.Subscribe("session")
	two := hub.Subscribe("session")
	one.Close()
	hub.Publish("session", Event{Sequence: 1, Payload: []byte("one")})
	if _, ok := <-one.Events; ok {
		t.Fatal("cancelled subscription remained open")
	}
	got := <-two.Events
	if got.Sequence != 1 || string(got.Payload) != "one" {
		t.Fatalf("unexpected event: %+v", got)
	}
}

func TestSlowSubscriberIsDroppedWithoutBlockingOthers(t *testing.T) {
	hub := New(1)
	slow := hub.Subscribe("session")
	fast := hub.Subscribe("session")
	hub.Publish("session", Event{Sequence: 1})
	if (<-fast.Events).Sequence != 1 {
		t.Fatal("fast subscriber missed first event")
	}
	hub.Publish("session", Event{Sequence: 2})
	if (<-fast.Events).Sequence != 2 {
		t.Fatal("fast subscriber missed second event")
	}
	hub.Publish("session", Event{Sequence: 3})
	if (<-fast.Events).Sequence != 3 {
		t.Fatal("fast subscriber missed third event")
	}
	if _, ok := <-slow.Events; !ok {
		t.Fatal("buffered event should be delivered before close")
	}
	if _, ok := <-slow.Events; ok {
		t.Fatal("slow subscriber should be closed")
	}
}
