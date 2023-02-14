package rrl_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/markdingo/rrl"
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

func newTuple(qClass, qType uint16, sName string, ac rrl.AllowanceCategory) *rrl.ResponseTuple {
	return &rrl.ResponseTuple{Class: qClass, Type: qType, AllowanceCategory: ac, SalientName: sName}
}

func TestNewAllowanceCategory(t *testing.T) {
	type testCase struct {
		rc, ac, nc int
		exp        rrl.AllowanceCategory
	}

	testCases := []testCase{
		{0, 1, 0, rrl.AllowanceAnswer},
		{0, 2, 1, rrl.AllowanceAnswer}, // nc should be ignored
		{0, 0, 1, rrl.AllowanceReferral},
		{0, 0, 2, rrl.AllowanceReferral},
		{0, 0, 0, rrl.AllowanceNoData},
		{3, 0, 0, rrl.AllowanceNXDomain},
		{3, 1, 0, rrl.AllowanceNXDomain},
		{3, 0, 1, rrl.AllowanceNXDomain},
		{3, 1, 1, rrl.AllowanceNXDomain},

		{1, 0, 0, rrl.AllowanceError},
		{2, 1, 0, rrl.AllowanceError},
		{4, 0, 1, rrl.AllowanceError},
		{5, 1, 1, rrl.AllowanceError},
	}

	for ix, tc := range testCases {
		ac := rrl.NewAllowanceCategory(tc.rc, tc.ac, tc.nc)
		if ac != tc.exp {
			t.Errorf("%d %d/%d/%d = %d(%s) Expected %d(%s)\n",
				ix, tc.rc, tc.ac, tc.nc, ac, ac, tc.exp, tc.exp)
		}
	}
}

// Make sure each AllowanceCategory responds to its corresponding config value
func TestAllowanceCategorysMatch(t *testing.T) {
	type testCase struct {
		ac rrl.AllowanceCategory
		w  string
	}

	testCases := []testCase{
		{rrl.AllowanceAnswer, "responses-per-second"},
		{rrl.AllowanceReferral, "referrals-per-second"},
		{rrl.AllowanceNoData, "nodata-per-second"},
		{rrl.AllowanceNXDomain, "nxdomains-per-second"},
		{rrl.AllowanceError, "errors-per-second"},
	}

	src := newAddr("udp", "127.0.0.1:53")
	for ix, tc := range testCases {
		cfg := rrl.NewConfig()
		cfg.SetNowFunc(func() time.Time {
			return time.Time{}
		})

		err := cfg.SetValue(tc.w, "1")
		if err != nil {
			t.Fatal(ix, "SetValue unexpectedly failed during setup", err)
		}
		R := rrl.NewRRL(cfg)
		act, ipr, qdr := R.Debit(src, newTuple(1, 1, "", tc.ac))
		if act != rrl.Send || ipr != rrl.IPNotConfigured || qdr != rrl.RTOk {
			t.Error(tc.ac, "should return Send, IPNotConfigure & RTOk, not", act, ipr, qdr)
		}
		act, ipr, qdr = R.Debit(src, newTuple(1, 1, "", tc.ac))
		if act != rrl.Drop || ipr != rrl.IPNotConfigured || qdr != rrl.RTRateLimit {
			t.Error(tc.ac, "should return rrl.Drop, IPNotConfigure & RTRateLimit, not", act, ipr, qdr)
		}
	}
}

func TestDebitIP(t *testing.T) {
	// A default config should let any query thru
	R := rrl.NewRRL(rrl.NewConfig())
	act, ipr, qdr := R.Debit(newAddr("udp", "127.0.0.1:53"), newTuple(1, 1, "", rrl.AllowanceAnswer))
	if act != rrl.Send || ipr != rrl.IPNotConfigured || qdr != rrl.RTNotConfigured {
		t.Error("Default Config should be Send, IPNotConfigured & RTNotConfigured, not", act, ipr, qdr)
	}

	cfg := rrl.NewConfig()
	err := cfg.SetValue("requests-per-second", "100")
	if err != nil {
		t.Fatal("SetValue failed during setup", err)
	}

	//  Use a custom clock to ensure the clock doesn't tick over more than a second
	// during the test run.
	var clock time.Time
	nowFunc := func() time.Time {
		return clock
	}
	cfg.SetNowFunc(nowFunc)
	R = rrl.NewRRL(cfg)

	type testCase struct {
		repeat int
		src    *addr
		qType  uint16
		qName  string
		zone   string
		wild   bool
		ac     rrl.AllowanceCategory

		eAction   rrl.Action
		eIPReason rrl.IPReason
		eRTReason rrl.RTReason
	}

	testCases := []testCase{
		// First 100 are within the config limit
		{100, newAddr("udp", "127.0.0.1:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Send, rrl.IPOk, rrl.RTNotConfigured}, // 0

		// Henceforth requests should be dropped as long as they are in the same network
		{1, newAddr("udp", "127.0.0.2:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Drop, rrl.IPRateLimit, rrl.RTNotReached}, // 1
		{1, newAddr("tcp", "127.0.0.3:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Drop, rrl.IPRateLimit, rrl.RTNotReached}, // 2
		{1, newAddr("udp4", "127.0.0.4:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Drop, rrl.IPRateLimit, rrl.RTNotReached}, // 3
		{1, newAddr("udp6", "127.0.0.5:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Drop, rrl.IPRateLimit, rrl.RTNotReached}, // 4
		{1, newAddr("udp", "127.0.0.6:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Drop, rrl.IPRateLimit, rrl.RTNotReached}, // 5

		// Different network should be accepted at first
		{100, newAddr("udp", "[::1]:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Send, rrl.IPOk, rrl.RTNotConfigured}, // 6
		{1, newAddr("udp", "[::2]:1000"), 1, "", "", false, rrl.AllowanceAnswer,
			rrl.Drop, rrl.IPRateLimit, rrl.RTNotReached}, // 7
	}

	// Because tests rely on previous settings we cannot use testing.Run() as that
	// would allow the running of individual tests that would then lack the previous
	// state they rely on.
	for ix, tc := range testCases {
		for rep := 0; rep < tc.repeat; rep++ {
			var fTime time.Time
			clock = fTime.Add(time.Hour)
			act, ipr, qdr := R.Debit(tc.src, newTuple(1, tc.qType, tc.qName, tc.ac))
			// fmt.Println("Res for", ix, rep, act, ipr, qdr)
			if act != tc.eAction {
				t.Errorf("%d Action Expected %s got %s tc %+v\n",
					ix, tc.eAction, act, tc)
			}
			if ipr != tc.eIPReason {
				t.Errorf("%d IPReason Expected %s got %s tc %+v\n",
					ix, tc.eIPReason, ipr, tc)
			}
			if qdr != tc.eRTReason {
				t.Errorf("%d RTReason Expected %s got %s tc %+v\n",
					ix, tc.eRTReason, qdr, tc)
			}
		}
	}
}

// Make sure slip-ratio works
func TestDebitSlip(t *testing.T) {
	cfg := rrl.NewConfig()
	cfg.SetNowFunc(func() time.Time {
		return time.Time{}
	})

	err := cfg.SetValue("responses-per-second", "1")
	if err != nil {
		t.Fatal("SetValue 'responses-per-second' unexpectedly failed during setup", err)
	}
	err = cfg.SetValue("slip-ratio", "2") // 1 in 2 should allow slip
	if err != nil {
		t.Fatal("SetValue 'slip-ratio' unexpectedly failed during setup", err)
	}
	R := rrl.NewRRL(cfg)

	src := newAddr("udp", "127.0.0.1:53")
	tuple := newTuple(1, 1, "example.com.", rrl.AllowanceAnswer)
	act, ipr, qdr := R.Debit(src, tuple)
	if act != rrl.Send || ipr != rrl.IPNotConfigured || qdr != rrl.RTOk {
		t.Error("Debit() should return Send, IPNotConfigure & RTOk, not", act, ipr, qdr)
	}
	act, ipr, qdr = R.Debit(src, tuple)
	if act != rrl.Drop || ipr != rrl.IPNotConfigured || qdr != rrl.RTRateLimit {
		t.Error("Debit() should return Send, IPNotConfigure & RTRateLimit, not", act, ipr, qdr)
	}
	act, ipr, qdr = R.Debit(src, tuple)
	if act != rrl.Slip || ipr != rrl.IPNotConfigured || qdr != rrl.RTRateLimit {
		t.Error("Debit() should return Slip, IPNotConfigure & RTRateLimit, not", act, ipr, qdr)
	}
	act, ipr, qdr = R.Debit(src, tuple)
	if act != rrl.Drop || ipr != rrl.IPNotConfigured || qdr != rrl.RTRateLimit {
		t.Error("Debit() should return Send, IPNotConfigure & RTRateLimit, not", act, ipr, qdr)
	}
}

func TestDebitUDPTCP(t *testing.T) {
	cfg := rrl.NewConfig()
	err := cfg.SetValue("responses-per-second", "1")
	if err != nil {
		t.Fatal("SetValue 'responses-per-second' unexpectedly failed during setup", err)
	}
	R := rrl.NewRRL(cfg)
	net1 := newAddr("udp", "127.0.0.1:53")
	net2 := newAddr("udp6", "127.0.0.1:53")
	net3 := newAddr("tcp", "127.0.0.1:53")
	tuple := newTuple(1, 1, "example.com.", rrl.AllowanceAnswer)
	act, ipr, qdr := R.Debit(net1, tuple)
	if act != rrl.Send {
		t.Fatal("Setup failed", act, ipr, qdr)
	}
	act, ipr, qdr = R.Debit(net2, tuple) // Still UDP, should now fail
	if act != rrl.Drop {
		t.Fatal("Expected rrl.Drop, not", act, ipr, qdr)
	}
	act, ipr, qdr = R.Debit(net3, tuple) // Non-UDP should succeed
	if act != rrl.Send {
		t.Fatal("TCP failed", act, ipr, qdr)
	}
}

func TestDebitIPCacheFull(t *testing.T) {
	cfg := rrl.NewConfig()
	err := cfg.SetValue("requests-per-second", "1")
	if err != nil {
		t.Fatal("SetValue 'requests-per-second' unexpectedly failed during setup", err)
	}

	// A table size of 1 actually gets turned into 4 * 256 shards by the caching layer as the
	// underlying cache always has 1024 shards and table size is the maximum depth per shard.
	err = cfg.SetValue("max-table-size", "1")
	if err != nil {
		t.Fatal("SetValue 'max-table-size' unexpectedly failed during setup", err)
	}
	//  Use a custom clock to ensure the clock doesn't tick over more than a second
	// during the test run. This clock never ticks over so eviction based on age
	// will/should never happen.
	var clock time.Time
	nowFunc := func() time.Time {
		return clock
	}
	cfg.SetNowFunc(nowFunc)
	R := rrl.NewRRL(cfg)
	var act rrl.Action
	var ipr rrl.IPReason
	tuple := newTuple(1, 1, "example.com.", rrl.AllowanceAnswer)

	// The iteration counts have been determined empirically. Typically about 500
	// entries will do the trick, depending on the hashing collisions in the shard
	// map.

	for ixa := 0; ixa < 10; ixa++ {
		for ixb := 0; ixb < 255; ixb++ {
			src := newAddr("udp", fmt.Sprintf("10.%d.%d.1:53", ixa, ixb))
			act, ipr, _ = R.Debit(src, tuple)
			if act != rrl.Send {
				break
			}
		}
	}

	if act != rrl.Drop && ipr != rrl.IPCacheFull {
		t.Error("Expected rrl.Drop due to IP cache full, but got", act, ipr)
	}
}

func TestDebitRTCacheFull(t *testing.T) {
	cfg := rrl.NewConfig()
	err := cfg.SetValue("responses-per-second", "1")
	if err != nil {
		t.Fatal("SetValue 'responses-per-second' unexpectedly failed during setup", err)
	}

	// A table size of 1 actually gets turned into 4 * 256 shards by the caching layer as the
	// underlying cache always has 1024 shards and table size is the maximum depth per shard.
	err = cfg.SetValue("max-table-size", "1")
	if err != nil {
		t.Fatal("SetValue 'max-table-size' unexpectedly failed during setup", err)
	}
	//  Use a custom clock to ensure the clock doesn't tick over more than a second
	// during the test run. This clock never ticks over so eviction based on age
	// will/should never happen.
	var clock time.Time
	nowFunc := func() time.Time {
		return clock
	}
	cfg.SetNowFunc(nowFunc)
	R := rrl.NewRRL(cfg)
	var act rrl.Action
	var rtr rrl.RTReason

	src := newAddr("udp", "127.0.0.1:53")

	// The iteration counts have been determined empirically. Typically about 500
	// entries will do the trick, depending on the hashing collision rate in the
	// shards.

	for ix := 0; ix < 1000; ix++ {
		tuple := newTuple(1, 1, fmt.Sprintf("%d.example.com.", ix), rrl.AllowanceAnswer)
		act, _, rtr = R.Debit(src, tuple)
		if act != rrl.Send {
			break
		}
	}

	if act != rrl.Drop && rtr != rrl.RTCacheFull {
		t.Error("Expected rrl.Drop due to cache full, but got", act, rtr)
	}
}

// Check that the account balance never goes below -15s
func TestDebitWindow(t *testing.T) {
	cfg := rrl.NewConfig()
	cfg.SetValue("responses-per-second", "1") // Set an easy-to-understand limit
	cfg.SetValue("window", "15")              // Always set just in case defaults change
	cfg.SetValue("slip-ratio", "0")

	//  Use a custom clock to ensure the clock doesn't tick over more than a second
	// during the test run. This clock never ticks over so eviction based on age
	// will/should never happen.
	clock := time.Now()
	nowFunc := func() time.Time {
		return clock
	}

	cfg.SetNowFunc(nowFunc)
	R := rrl.NewRRL(cfg)

	src := newAddr("udp", "127.0.0.1:53")
	tuple := newTuple(1, 1, "example.com.", rrl.AllowanceAnswer)

	act, _, _ := R.Debit(src, tuple) // Consume our one second of credit
	if act != rrl.Send {
		t.Fatal("First debit should have allowed Send")
	}

	for ix := 0; ix < 20; ix++ { // More than 15 shouldn't hurt
		act, _, _ = R.Debit(src, tuple) // Make sure we're now dropped
		if act != rrl.Drop {
			t.Fatal(ix, "Expected switch to negative credit", act)
		}
	}

	// Should now have the full 15s of negative credits.

	clock = clock.Add(time.Second * 14)
	act, _, _ = R.Debit(src, tuple) // Consume our one second of credit - should be -13s
	if act != rrl.Drop {
		t.Fatal("Clock+ 14s should not have given credits")
	}

	clock = clock.Add(time.Second * 4) // Should now be back in positive territory by +1s
	act, _, _ = R.Debit(src, tuple)    // Consume our one second of credit
	if act != rrl.Send {
		t.Fatal("Clock+ 4s should have given credits")
	}

	act, _, _ = R.Debit(src, tuple) // Did we get too many credits?
	if act == rrl.Send {
		t.Fatal("Clock+ 3s gave too many credits")
	}
}
