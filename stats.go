package rrl

import (
	"fmt"
)

// Stats tracks basic statistics - mostly the results of Debit calls.
// Callers are responsible for concurrency protection if needed.
// Normal access to Stats is via [GetStats] which populates cache values and
// provides concurrency protection.
type Stats struct {
	RPS       [AllowanceLast]int64 // Since last zero
	Actions   [ActionLast]int64
	IPReasons [IPLast]int64
	RTReasons [RTLast]int64

	CacheLength int   // Always current
	Evictions   int64 // Since last zero
}

var zero Stats

// Copy returns a copy of the current stats and optionally zeroes the source afterwards.
func (c *Stats) Copy(zeroAfter bool) (ret Stats) {
	ret = *c

	if zeroAfter {
		*c = zero
	}

	return
}

// Add assumes any concurrency protection required is managed by the caller.
func (c *Stats) Add(from *Stats) {
	for ix, v := range from.RPS {
		c.RPS[ix] += v
	}
	for ix, v := range from.Actions {
		c.Actions[ix] += v
	}
	for ix, v := range from.IPReasons {
		c.IPReasons[ix] += v
	}
	for ix, v := range from.RTReasons {
		c.RTReasons[ix] += v
	}
	c.CacheLength = from.CacheLength // Would max() or avg() be more useful?
	c.Evictions += from.Evictions
}

// IncrementDebit bumps all stats affected by a Debit call.
func (c *Stats) incrementDebit(act Action, ipr IPReason, rtr RTReason, ac AllowanceCategory) {
	if act >= 0 && act < ActionLast {
		c.Actions[act]++
	}
	if ipr >= 0 && ipr < IPLast {
		c.IPReasons[ipr]++
	}
	if rtr >= 0 && rtr < RTLast {
		c.RTReasons[rtr]++
	}
	if ac >= 0 && ac < AllowanceLast {
		c.RPS[ac]++
	}
}

func (c *Stats) String() string {
	return fmt.Sprintf("RPS %d/%d/%d/%d/%d Actions %d/%d/%d IPR %d/%d/%d/%d/%d RTR %d/%d/%d/%d/%d/%d L=%d/%d",
		c.RPS[AllowanceAnswer], c.RPS[AllowanceReferral], c.RPS[AllowanceNoData], c.RPS[AllowanceNXDomain],
		c.RPS[AllowanceError],
		c.Actions[Send], c.Actions[Drop], c.Actions[Slip],
		c.IPReasons[IPOk], c.IPReasons[IPNotConfigured], c.IPReasons[IPNotReached], c.IPReasons[IPRateLimit],
		c.IPReasons[IPCacheFull],
		c.RTReasons[RTOk], c.RTReasons[RTNotConfigured], c.RTReasons[RTNotReached], c.RTReasons[RTRateLimit],
		c.RTReasons[RTNotUDP], c.RTReasons[RTCacheFull],
		c.CacheLength, c.Evictions)
}
