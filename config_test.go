package rrl_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/markdingo/rrl"
)

func TestConfigDefault(t *testing.T) {
	cfg := rrl.NewConfig()
	if cfg == nil {
		t.Fatal("Should have a *config")
	}
	got := cfg.String()
	exp := "15000000000 24-56 0/0/0/0/0/0 2/100000 false/false/false/false"
	if exp != got {
		t.Error("Default Config is", got, "but expected", exp)
	}

	// Cause the config to be finalized and thus some internal values change
	r := rrl.NewRRL(cfg)
	if r == nil {
		t.Fatal("Should have a *RRL")
	}
	got = cfg.String()
	exp = "15000000000 24-56 0/0/0/0/0/0 2/100000 false/false/false/false"
	if exp != got {
		t.Error("Finalized zero Config is", got, "but expected", exp)
	}

	// Change one value just to ensure it propagates thru finalize
	cfg.SetValue("responses-per-second", "7")
	r = rrl.NewRRL(cfg)
	got = cfg.String()
	exp = "15000000000 24-56 142857142/142857142/142857142/142857142/142857142/0 2/100000 false/false/false/false"
	if exp != got {
		t.Error("Finalized non-zero Config is", got, "but expected", exp)
	}

	newR := rrl.RRL{}
	_ = newR
}

func TestConfigSet(t *testing.T) {
	cfg := rrl.NewConfig()
	testCases := []struct {
		w    string
		arg  string
		emsg string
	}{
		// Bad settings
		{"windox", "", "unknown"},

		{"window", "x23", "invalid syntax"},
		{"window", "-1", "between"},
		{"window", "1", ""},

		{"ipv4-prefix-length", "-1", "be between"},
		{"ipv4-prefix-length", "33", "be between"},
		{"ipv4-prefix-length", "24", ""},
		{"ipv4-prefix-length", "x24", "invalid syntax"},

		{"ipv6-prefix-length", "-1", "be between"},
		{"ipv6-prefix-length", "129", "be between"},
		{"ipv6-prefix-length", "xx129", "syntax"},
		{"ipv6-prefix-length", "64", ""},

		{"responses-per-second", "-1", "negative"},
		{"responses-per-second", "xxy", "invalid syntax"},
		{"responses-per-second", "0", ""},
		{"responses-per-second", "2", ""},

		{"nodata-per-second", "-1", "negative"},
		{"nodata-per-second", "abc", "syntax"},
		{"nodata-per-second", "3", ""},

		{"nxdomains-per-second", "-1", "negative"},
		{"nxdomains-per-second", "xyz", "syntax"},
		{"nxdomains-per-second", "4", ""},

		{"referrals-per-second", "-1", "negative"},
		{"referrals-per-second", "xyz", "syntax"},
		{"referrals-per-second", "5.55", ""},
		{"referrals-per-second", "5", ""},

		{"errors-per-second", "-1", "negative"},
		{"errors-per-second", "xyz", "syntax"},
		{"errors-per-second", "6.001", ""},
		{"errors-per-second", "6", ""},

		{"requests-per-second", "-1", "negative"},
		{"requests-per-second", "xx", "syntax"},
		{"requests-per-second", "7", ""},

		{"slip-ratio", "-1", "be between"},
		{"slip-ratio", "ccc", "syntax"},
		{"slip-ratio", "8", ""},

		{"max-table-size", "-1", "negative"},
		{"max-table-size", "xx", "syntax"},
		{"max-table-size", "9", ""},
	}

	for ix, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", ix, tc.w),
			func(tt *testing.T) {
				err := cfg.SetValue(tc.w, tc.arg)
				if err != nil {
					if len(tc.emsg) == 0 {
						tt.Error("Didn't expect error of", err.Error())
						return
					}
					if !strings.Contains(err.Error(), tc.emsg) {
						tt.Errorf("Expected '%s' in %s\n", tc.emsg, err.Error())
						return
					}
					return
				}

				if len(tc.emsg) > 0 {
					tt.Error("Expected an error return containing", tc.emsg)
					return
				}
			})
	}

	// Look at the internal values of config to see if they were all set
	got := cfg.String()
	exp := "1000000000 24-64 500000000/333333333/250000000/200000000/166666666/142857142 8/9 true/true/true/true"
	if got != exp {
		t.Error("Config is", got, "but expected", exp)
	}
}
