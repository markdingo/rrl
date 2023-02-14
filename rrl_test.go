package rrl

import (
	"testing"
)

func TestAllowanceForRtype(t *testing.T) {
	cfg := NewConfig()
	cfg.SetValue("responses-per-second", "1")
	R := NewRRL(cfg)
	at := R.allowanceForRtype(AllowanceAnswer)
	if at != 1*second {
		t.Error("AllowanceAnswer should be 1, not", at)
	}
}

func TestAddrPrefix(t *testing.T) {
	type testCase struct {
		addr   string
		expect string
		four   string
		six    string
	}

	testCases := []testCase{
		{"127.0.0.1", "", "", ""},
		{"127.0.0.1:50", "127.0.0.0", "", ""},
		{"127.1.2.1:50", "127.1.2.0", "", ""},
		{"127.1.2.1:50", "127.0.0.0", "8", ""},
		{"127.1.2.1:50", "127.1.0.0", "16", ""},

		{"[::", "", "", ""},
		{"[::]", "", "", ""},
		{"[::1]:53", "::", "", ""},
		{"[::ff]:53", "::", "", ""},
		{"[::1:2:3:4:5:6]:53", "0:0:1::", "", ""},
		{"[::1:2:3:4:5:6]:53", "0:0:1:2::", "", "64"},
		{"", "", "", ""},
	}

	for ix, tc := range testCases {

		cfg := NewConfig()
		if len(tc.four) > 0 {
			cfg.SetValue("ipv4-prefix-length", tc.four)
		}
		if len(tc.six) > 0 {
			cfg.SetValue("ipv6-prefix-length", tc.six)
		}
		R := NewRRL(cfg)
		got := R.addrPrefix(tc.addr)
		if got != tc.expect {
			t.Error(ix, "addrPrefix failed Expected", tc.expect, "got", got, "from", tc.addr)
		}
	}
}
