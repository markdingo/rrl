package rrl

import (
	"fmt"
	"strconv"
	"time"
)

const second = 1000000000 // Equals time.Second - maybe config variables should be time.Duration?

// Config provides the variable settings for an RRL.
// A Config should only ever be created with [NewConfig] as it requires non-zero default
// values.
// All Config values are set using the [SetValue] function.
//
// A default config is effectively a no-op as most values default to responses-per-second
// which itself defaults to zero. The isActive() function returns true if the Config
// contains values which cause RRL to apply debit rules.
//
// Unset values which default to responses-per-second are set when the Config is passed to
// [NewRRL].
//
// All values are either an unsigned int (as accepted by [strconv.ParseUint]) an unsigned
// float (as accepted by [strconv.ParseFloat]).
//
// The following keywords are accepted:
//
// window int SECONDS - the rolling window in SECONDS during which response rates are
// tracked.
// Default 15.
//
// ipv4-prefix-length int LENGTH - the prefix LENGTH in bits to use for identifying a ipv4
// client CIDR.
// Default 24.
//
// ipv6-prefix-length int LENGTH - the prefix LENGTH in bits to use for identifying a ipv6
// client CIDR.
// Default 56.
//
// responses-per-second float ALLOWANCE - the number AllowanceAnswer responses allowed per
// second.
// An ALLOWANCE of 0 disables rate limiting.
// Default 0.
//
// nodata-per-second float ALLOWANCE - the number of AllowanceNoData responses allowed per second.
// An ALLOWANCE of 0 disables rate limiting.
// Defaults to responses-per-second.
//
// nxdomains-per-second float ALLOWANCE - the number of AllowanceNXDomain responses allowed per
// second.
// An ALLOWANCE of 0 disables rate limiting.
// Defaults to responses-per-second.
//
// referrals-per-second float ALLOWANCE - the number of AllowanceReferral responses allowed per
// second.
// An ALLOWANCE of 0 disables rate limiting.
// Defaults to responses-per-second.
//
// errors-per-second float ALLOWANCE - the number of AllowanceError allowed per second
// (excluding NXDOMAIN).
// An ALLOWANCE of 0 disables rate limiting.
// Defaults to responses-per-second.
//
// requests-per-second float ALLOWANCE - the number of requests allowed per second from source
// IP.
// An ALLOWANCE of 0 disables rate limiting of requests.
// This value applies solely to the claimed source IP of the query whereas all other
// settings apply to response details.
// Default 0.
//
// max-table-size int SIZE - the maximum number of responses to be tracked at one time.
// When exceeded, rrl stops rate limiting new responses.
// Defaults to 100000.
//
// slip-ratio int RATIO - the ratio of rate-limited responses which are given a truncated
// response over a dropped response.
// A RATIO of 0 disables slip processing and thus all rate-limited responses will be dropped.
// A RATIO of 1 means every rate-limited response will be a truncated response and the
// upper limit of 10 means 1 in every 10 rate-limited responses will be a truncated with
// the remaining 9 being dropped.
// Default is 2.
//
// For those wishing to examine the internal values, with the String() function, note that
// while intervals are set as per-second values they are internally converted to the
// number of nanoseconds to decrement per Debit call, so expect the unexpected.
//
// ISC config values not yet supported by this package are: qps-scale and
// all-per-second. Maybe one day...
type Config struct {
	window int64

	ipv4PrefixLength int
	ipv6PrefixLength int

	responsesInterval int64
	nodataInterval    int64
	nxdomainsInterval int64
	referralsInterval int64
	errorsInterval    int64
	requestsInterval  int64

	slipRatio    uint
	maxTableSize int

	// Managed by Set() and checked by finalize()
	nodataIntervalSet    bool
	nxdomainsIntervalSet bool
	referralsIntervalSet bool
	errorsIntervalSet    bool

	nowFunc func() time.Time // Used by tests to control clock
}

// These defaults largely reflect those recommended by ISC.
var defaultConfig = Config{
	window:           15 * second,
	ipv4PrefixLength: 24,
	ipv6PrefixLength: 56,
	slipRatio:        2,
	maxTableSize:     100000,
	nowFunc:          time.Now,
}

// NewConfig returns a new Config struct with all the default values set. This is the only
// way you should ever create a Config.
func NewConfig() *Config {
	c := defaultConfig // Take a copy

	return &c
}

// IsActive returns true if at least one of the intervals is set and thus causes Debit to
// evaluate accounts. IOWs it returns !no-op.
func (c *Config) IsActive() bool {
	return c.responsesInterval > 0 || c.nodataInterval > 0 || c.nxdomainsInterval > 0 || c.referralsInterval > 0 || c.errorsInterval > 0 || c.requestsInterval > 0
}

// argInvalidErr is a helper function for Set() to generate a common error when the
// argument value supplied is invalid for reasons such as it cannot be parsed as a number
// or is outside the valid range.
func argInvalidErr(keyword, val string, em interface{}) error {
	if t, ok := em.(error); ok {
		return fmt.Errorf("%s='%s' %w", keyword, val, t)
	}

	return fmt.Errorf("%s='%s' %s", keyword, val, em)
}

// SetValue changes the configuration values for the nominated keyword [Config].
//
// SetValue is provided as a keyword-based setter to try and make it compatible with the
// original coredns/rrl plugin as possible.
// Serendipitously, this should also assist programs which use [https://pkg.go.dev/flag]
// with the keywords as option names such as --window xx.
//
// Note that only keywords specific to this standalone rrl package have been carried over
// from coredns.
// For example "report-only" is not handled here as it is now expected to be handled by
// the caller as part of the design goal of decoupling rrl from anything specific to
// coredns.
//
// See [Config] for a full list of valid keywords.
//
// Example:
//
//	c := NewConfig()
//	c.SetValue("window", "30")
func (c *Config) SetValue(keyword string, arg string) error {
	switch keyword {
	case "window":
		w, err := strconv.Atoi(arg)
		if err != nil {
			return argInvalidErr(keyword, arg, err)
		}
		if w <= 0 || w > 3600 { // One second to one hour
			return argInvalidErr(keyword, arg, "window must be between 1 and 3600")
		}
		c.window = int64(w * second)

	case "ipv4-prefix-length":
		i, err := strconv.Atoi(arg)
		if err != nil {
			return argInvalidErr(keyword, arg, err)
		}
		if i <= 0 || i > 32 {
			return argInvalidErr(keyword, arg, "must be between 1 and 32")
		}
		c.ipv4PrefixLength = i

	case "ipv6-prefix-length":
		i, err := strconv.Atoi(arg)
		if err != nil {
			return argInvalidErr(keyword, arg, err)
		}
		if i <= 0 || i > 128 {
			return argInvalidErr(keyword, arg, "must be between 1 and 128")
		}
		c.ipv6PrefixLength = i

	case "responses-per-second":
		i, err := getIntervalArg(keyword, arg)
		if err != nil {
			return err
		}
		c.responsesInterval = i

	case "nodata-per-second":
		i, err := getIntervalArg(keyword, arg)
		if err != nil {
			return err
		}
		c.nodataInterval = i
		c.nodataIntervalSet = true

	case "nxdomains-per-second":
		i, err := getIntervalArg(keyword, arg)
		if err != nil {
			return err
		}
		c.nxdomainsInterval = i
		c.nxdomainsIntervalSet = true

	case "referrals-per-second":
		i, err := getIntervalArg(keyword, arg)
		if err != nil {
			return err
		}
		c.referralsInterval = i
		c.referralsIntervalSet = true

	case "errors-per-second":
		i, err := getIntervalArg(keyword, arg)
		if err != nil {
			return err
		}
		c.errorsInterval = i
		c.errorsIntervalSet = true

	case "slip-ratio":
		i, err := strconv.Atoi(arg)
		if err != nil {
			return argInvalidErr(keyword, arg, err)
		}
		if i < 0 || i > 10 {
			return argInvalidErr(keyword, arg, "must be between 0 and 10")
		}
		c.slipRatio = uint(i)

	case "requests-per-second":
		i, err := getIntervalArg(keyword, arg)
		if err != nil {
			return err
		}
		c.requestsInterval = i

	case "max-table-size":
		i, err := strconv.Atoi(arg)
		if err != nil {
			return argInvalidErr(keyword, arg, err)
		}
		if i < 0 {
			return argInvalidErr(keyword, arg, "cannot be negative")
		}
		c.maxTableSize = i

	default:
		return fmt.Errorf("unknown Set() keyword '%v'", keyword)
	}

	return nil
}

// SetNowFunc is intended for testing purposes only. It replaces the time.Now() function
// used in the cache eviction logic.
func (c *Config) SetNowFunc(fn func() time.Time) {
	c.nowFunc = fn
}

// finalize is called after all config values have been set as part of the config being
// imported into the RRL. If any allowance intervals are not set, default them to
// responsesInterval which may itself not be set...
// Also set the now func if that has not already been set.
func (c *Config) finalize() {
	if !c.nodataIntervalSet {
		c.nodataInterval = c.responsesInterval
	}
	if !c.nxdomainsIntervalSet {
		c.nxdomainsInterval = c.responsesInterval
	}
	if !c.referralsIntervalSet {
		c.referralsInterval = c.responsesInterval
	}
	if !c.errorsIntervalSet {
		c.errorsInterval = c.responsesInterval
	}

	if c.nowFunc == nil {
		c.nowFunc = time.Now
	}
}

// getIntervalArg is a helper function to convert a string into a loating point which in
// turn is converted into the number of nanoseconds to add to the allowTime for each query.
func getIntervalArg(keyword string, arg string) (int64, error) {
	rps, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return 0, argInvalidErr(keyword, arg, err)
	}
	if rps < 0 {
		return 0, argInvalidErr(keyword, arg, "cannot be negative")
	}
	if rps == 0.0 {
		return 0, nil
	} else {
		return int64(second / rps), nil
	}
}

// String is mainly intended for test code so it can verify internal values without having
// direct access to them.
// Of course the caller is free to use this printable value too.
//
// The returned string is a single line of text containing all config values with
// all per-second values expressed as nanoseconds decrements.
func (c *Config) String() string {
	return fmt.Sprintf("%d %d-%d %d/%d/%d/%d/%d/%d %d/%d %t/%t/%t/%t",
		c.window,
		c.ipv4PrefixLength, c.ipv6PrefixLength,
		c.responsesInterval, c.nodataInterval, c.nxdomainsInterval, c.referralsInterval, c.errorsInterval, c.requestsInterval,
		c.slipRatio, c.maxTableSize,
		c.nodataIntervalSet, c.nxdomainsIntervalSet, c.referralsIntervalSet, c.errorsIntervalSet)
}
