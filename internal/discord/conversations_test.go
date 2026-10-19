package discord

import (
	"fmt"
	"testing"
	"time"
)

func TestConversationIsolationExpiryAndBounds(t *testing.T) {
	var c conversations
	now := time.Now()
	key := conversationKey("g", "c", "u")
	for i := 0; i < 6; i++ {
		c.add(key, []string{"b", "a"}, fmt.Sprint(i), "answer", now)
	}
	got := c.read(key, []string{"a", "b"}, now)
	if len(got) != 4 || got[0].Question != "2" {
		t.Fatal(got)
	}
	if len(c.read(conversationKey("g", "c", "other"), []string{"a", "b"}, now)) != 0 {
		t.Fatal("cross-user conversation")
	}
	if len(c.read(key, []string{"a"}, now)) != 0 {
		t.Fatal("revoked permissions leaked history")
	}
	c.add(key, []string{"a"}, "q", "a", now)
	if len(c.read(key, []string{"a"}, now.Add(16*time.Minute))) != 0 {
		t.Fatal("expired history")
	}
	for i := 0; i < 200; i++ {
		c.add(fmt.Sprint(i), []string{"a"}, "q", "a", now)
	}
	if len(c.entries) != 128 {
		t.Fatal("unbounded cache")
	}
}

func TestDeletedSourceInvalidatesFollowups(t *testing.T) {
	var c conversations
	now := time.Now()
	c.add("one", []string{"source"}, "question", "answer", now)
	c.add("two", []string{"elsewhere"}, "question", "answer", now)
	c.invalidateChannel("source")
	if len(c.read("one", []string{"source"}, now)) != 0 || len(c.read("two", []string{"elsewhere"}, now)) != 1 {
		t.Fatal("incorrect invalidation")
	}
}
