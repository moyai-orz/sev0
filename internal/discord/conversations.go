package discord

import (
	"slices"
	"strings"
	"sync"
	"time"
)

type exchange struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
type conversation struct {
	Turns   []exchange
	Expires time.Time
	Access  string
}
type conversations struct {
	mu      sync.Mutex
	entries map[string]conversation
}

func conversationKey(guild, channel, user string) string { return guild + ":" + channel + ":" + user }
func accessKey(channels []string) string {
	copy := slices.Clone(channels)
	slices.Sort(copy)
	return strings.Join(copy, ",")
}

func (c *conversations) read(key string, channels []string, now time.Time) []exchange {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}
	// Never carry old answers across a change in visible channels.
	if !now.Before(entry.Expires) || entry.Access != accessKey(channels) {
		delete(c.entries, key)
		return nil
	}
	return slices.Clone(entry.Turns)
}
func (c *conversations) add(key string, channels []string, question, answer string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]conversation)
	}
	for k, v := range c.entries {
		if !now.Before(v.Expires) {
			delete(c.entries, k)
		}
	}
	access := accessKey(channels)
	entry := c.entries[key]
	if entry.Access != access {
		entry.Turns = nil
	}
	if _, ok := c.entries[key]; !ok && len(c.entries) >= 128 {
		oldestKey := ""
		var oldest time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.Expires.Before(oldest) {
				oldestKey = k
				oldest = v.Expires
			}
		}
		delete(c.entries, oldestKey)
	}
	entry.Turns = append(entry.Turns, exchange{Question: truncateDiscord(question, 1000), Answer: truncateDiscord(answer, 2000)})
	if len(entry.Turns) > 4 {
		entry.Turns = slices.Clone(entry.Turns[len(entry.Turns)-4:])
	}
	entry.Access = access
	entry.Expires = now.Add(15 * time.Minute)
	c.entries[key] = entry
}

func (c *conversations) invalidateChannel(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if slices.Contains(strings.Split(entry.Access, ","), channel) {
			delete(c.entries, key)
		}
	}
}
