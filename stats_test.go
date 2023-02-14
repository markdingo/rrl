package rrl

import (
	"fmt"
	"testing"
	"time"
)

// addr implements a net.Addr
type addr struct {
	n, s string
}

func (a *addr) Network() string { return a.n }
func (a *addr) String() string  { return a.s }

func newAddr(n, s string) *addr {
	return &addr{n: n, s: s}
}

func newTuple(qClass, qType uint16, sName string, ac AllowanceCategory) *ResponseTuple {
	return &ResponseTuple{Class: qClass, Type: qType, AllowanceCategory: ac, SalientName: sName}
}

func TestStatsBasics(t *testing.T) {
	c := Stats{}

	s := c.String()
	exp := "RPS 0/0/0/0/0 Actions 0/0/0 IPR 0/0/0/0/0 RTR 0/0/0/0/0/0 L=0/0"
	if s != exp {
		t.Error("Zero stats expected", exp, "got", s)
	}

	c.incrementDebit(Send, IPOk, RTOk, AllowanceAnswer)
	s = c.String()
	exp = "RPS 1/0/0/0/0 Actions 1/0/0 IPR 1/0/0/0/0 RTR 1/0/0/0/0/0 L=0/0"
	if s != exp {
		t.Error("Non-zero stats expected", exp, "got", s)
	}

	c.incrementDebit(Slip, IPCacheFull, RTCacheFull, AllowanceError)
	s = c.String()
	exp = "RPS 1/0/0/0/1 Actions 1/0/1 IPR 1/0/0/0/1 RTR 1/0/0/0/0/1 L=0/0"
	if s != exp {
		t.Error("Trailing non-zero stats expected", exp, "got", s)
	}

	copy := c.Copy(false)
	s = copy.String()
	if s != exp {
		t.Error("Copy stats expected", exp, "got", s)
	}
	s = c.String()
	if s != exp {
		t.Error("Post-copy stats expected", exp, "got", s)
	}

	c.Copy(true)
	s = c.String()
	exp = "RPS 0/0/0/0/0 Actions 0/0/0 IPR 0/0/0/0/0 RTR 0/0/0/0/0/0 L=0/0"
	if s != exp {
		t.Error("Post-copy stats expected", exp, "got", s)
	}
}

func TestStatsViaRRL(t *testing.T) {
	cfg := NewConfig()
	cfg.SetValue("responses-per-second", "10")
	cfg.SetValue("requests-per-second", "10") // by IP
	R := NewRRL(cfg)
	src := newAddr("udp", "127.0.0.1:53")
	R.Debit(src, newTuple(1, 1, "example.com.", AllowanceAnswer))
	c := R.GetStats(true)
	s := c.String()
	exp := "RPS 1/0/0/0/0 Actions 1/0/0 IPR 1/0/0/0/0 RTR 1/0/0/0/0/0 L=2/0"
	if s != exp {
		t.Error("Non-zero stats expected", exp, "got", s)
	}

	// Zero zeroes out all counters, but the cache length remains unchanged as it
	// always reflects the current value.
	c = R.GetStats(true)
	s = c.String()
	exp = "RPS 0/0/0/0/0 Actions 0/0/0 IPR 0/0/0/0/0 RTR 0/0/0/0/0/0 L=2/0"
	if s != exp {
		t.Error("Zero stats expected", exp, "got", s)
	}
}

func TestEvictionStats(t *testing.T) {
	cfg := NewConfig()
	cfg.SetValue("windows", "1")
	cfg.SetValue("responses-per-second", "10")
	cfg.SetValue("requests-per-second", "1") // by IP
	var clock time.Time
	nowFunc := func() time.Time {
		return clock
	}
	cfg.SetNowFunc(nowFunc)
	R := NewRRL(cfg)
	tuple := newTuple(1, 1, "example.com.", AllowanceAnswer)

	// The iteration counts have been determined empirically. Typically about 500
	// entries will do the trick, depending on the hashing collisions in the shard
	// map.

	for ixa := 0; ixa < 200; ixa++ {
		for ixb := 0; ixb < 255; ixb++ {
			src := newAddr("udp", fmt.Sprintf("10.%d.%d.1:53", ixa, ixb))
			R.Debit(src, tuple)
			clock = clock.Add(time.Second)
		}
	}

	c := R.GetStats(false)
	if c.Evictions == 0 {
		t.Error("Expected some evictions, but got", c.String())
	}
}

func TestStatsAdd(t *testing.T) {
	var a, b Stats

	a.RPS[0] = 1
	a.Actions[1] = 6
	a.Actions[2] = 7
	a.IPReasons[2] = 2
	a.RTReasons[1] = 3
	a.CacheLength = 4
	a.Evictions = 5
	b.Add(&a)
	b.CacheLength = 0
	b.Add(&a)

	got := b.String()
	exp := "RPS 2/0/0/0/0 Actions 0/12/14 IPR 0/0/4/0/0 RTR 0/6/0/0/0/0 L=4/10"
	if got != exp {
		t.Error("Exp", exp, "Got", got)
	}
}
