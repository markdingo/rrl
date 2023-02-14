package rrl

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/markdingo/rrl/cache"
)

// RRL contains the configuration and "account" database.
// An RRL is safe for concurrent use by multiple goroutines.
type RRL struct {
	cfg   Config
	table *cache.Cache

	statsMu sync.Mutex
	stats   Stats
}

// NewRRL creates a new RRL struct which is ready for use.
// The config parameter is created by the [NewConfig] and [SetValue] functions.
// All config default values are set by NewRRL and are visible in the Config
// on return.
// NewRRL takes a copy of Config so subsequent changes have no effect on the RRL.
func NewRRL(cfg *Config) *RRL {
	cfg.finalize()         // Finalize the caller's copy
	rrl := &RRL{cfg: *cfg} // But make our own copy so caller cannot modify
	rrl.initTable()

	return rrl
}

// responseAccount holds accounting for a category of response
type responseAccount struct {
	allowTime     int64 // Next response is allowed if current time >= allowTime
	slipCountdown uint  // When at 1, a dropped response slips through instead of being dropped
}

// allowanceForRtype returns the configured response interval for the indicated response
// type.
// Different response types have their own configuration limits.
func (rrl *RRL) allowanceForRtype(rt AllowanceCategory) int64 {
	switch rt {
	case AllowanceAnswer:
		return rrl.cfg.responsesInterval
	case AllowanceNoData:
		return rrl.cfg.nodataInterval
	case AllowanceNXDomain:
		return rrl.cfg.nxdomainsInterval
	case AllowanceReferral:
		return rrl.cfg.referralsInterval
	case AllowanceError:
		return rrl.cfg.errorsInterval
	}
	return -1 // Unknown response - odd
}

// initTable creates a new cache table and sets the cache eviction function
func (rrl *RRL) initTable() {
	rrl.table = cache.New(rrl.cfg.maxTableSize)
	// This eviction function returns true if the allowance is >= max value (window)
	rrl.table.SetEvict(func(el interface{}) bool {
		ra, ok := (el).(*responseAccount)
		if !ok {
			return true
		}
		evicted := rrl.cfg.nowFunc().UnixNano()-ra.allowTime >= rrl.cfg.window
		if evicted {
			rrl.incrementEviction()
		}
		return evicted
	})
}

// accountToken returns a token string for the query details and indicated AllowanceCategory
func (rrl *RRL) accountToken(ipPrefix string, qType uint16, name string, rt AllowanceCategory) string {
	return rrl.buildToken(rt, qType, strings.ToLower(name), ipPrefix)
}

// buildToken returns a token string for the given inputs
func (rrl *RRL) buildToken(rt AllowanceCategory, qType uint16, name, ipPrefix string) string {
	// "Per BIND" references below are copied from the BIND 9.11 Manual
	// https://ftp.isc.org/isc/bind9/cur/9.11/doc/arm/Bv9ARM.pdf
	rtypestr := strconv.FormatUint(uint64(rt), 10)
	switch rt {
	case AllowanceAnswer:
		// Per BIND: All non-empty responses for a valid domain name (qname) and record type (qType) are identical
		qTypeStr := strconv.FormatUint(uint64(qType), 10)
		return strings.Join([]string{ipPrefix, rtypestr, qTypeStr, name}, "/")
	case AllowanceNoData:
		// Per BIND: All empty (NODATA) responses for a valid domain, regardless of query type, are identical.
		return strings.Join([]string{ipPrefix, rtypestr, "", name}, "/")
	case AllowanceNXDomain:
		// Per BIND: Requests for any and all undefined subdomains of a given valid domain result in NXDOMAIN errors
		// and are identical regardless of query type.
		return strings.Join([]string{ipPrefix, rtypestr, "", name}, "/")
	case AllowanceReferral:
		// Per BIND: Referrals or delegations to the server of a given domain are identical.
		qTypeStr := strconv.FormatUint(uint64(qType), 10)
		return strings.Join([]string{ipPrefix, rtypestr, qTypeStr, name}, "/")
	case AllowanceError:
		// Per BIND: All requests that result in DNS errors other than NXDOMAIN, such as SERVFAIL and FORMERR, are
		// identical regardless of requested name (qname) or record type (qType).
		return strings.Join([]string{ipPrefix, rtypestr, "", ""}, "/")
	}
	return ""
}

// debit updates an existing response account in the rrl table and recalculate the current
// balance, or if the response account does not exist, it will add it.
//
// Return values are Balance, slip and error.
func (rrl *RRL) debit(allowance int64, t string) (int64, bool, error) {

	type balances struct {
		balance int64
		slip    bool
	}

	result := rrl.table.UpdateAdd(t,
		// the 'update' function updates the account and returns the new balance
		func(el interface{}) interface{} {
			ra := (el).(*responseAccount)
			if ra == nil {
				return nil
			}
			now := rrl.cfg.nowFunc().UnixNano()
			balance := now - ra.allowTime - allowance
			if balance >= int64(time.Second) {
				// positive balance can't exceed 1 second
				balance = int64(time.Second) - allowance
			} else if balance < -rrl.cfg.window {
				// balance can't be more negative than window
				balance = -rrl.cfg.window
			}
			ra.allowTime = now - balance
			if balance > 0 || ra.slipCountdown == 0 {
				return balances{balance, false}
			}
			if ra.slipCountdown == 1 {
				ra.slipCountdown = rrl.cfg.slipRatio
				return balances{balance, true}
			}
			ra.slipCountdown -= 1
			return balances{balance, false}

		},
		// The 'add' function create a new account for the token. allowTime is
		// given a credit of one second worth of queries less the allowance for
		// the current query.
		func() interface{} {
			ra := &responseAccount{
				allowTime:     rrl.cfg.nowFunc().UnixNano() - int64(time.Second) + allowance,
				slipCountdown: rrl.cfg.slipRatio,
			}
			return ra
		})

	if result == nil {
		return 0, false, nil
	}
	if err, ok := result.(error); ok {
		return 0, false, err
	}
	if b, ok := result.(balances); ok {
		return b.balance, b.slip, nil
	}
	return 0, false, errors.New("unexpected result type")
}

// addrPrefix returns the address prefix of the net.Addr style address string
// (e.g. 1.2.3.4:1234 or [1:2::3:4]:1234) based on the configured prefix lengths.
func (rrl *RRL) addrPrefix(addr string) string {
	i := strings.LastIndex(addr, ":")
	if i < 4 { // Shortest valid index for "[::]:1" is 4
		return ""
	}
	ip := net.ParseIP(addr[:i])
	if ip.To4() != nil {
		ip = ip.Mask(net.CIDRMask(rrl.cfg.ipv4PrefixLength, 32))
		return ip.String()
	}
	ip = net.ParseIP(addr[1 : i-1]) // strip brackets from ipv6 e.g. [2001:db8::1]
	ip = ip.Mask(net.CIDRMask(rrl.cfg.ipv6PrefixLength, 128))

	return ip.String()
}

// Args must be pass-by-reference because pass-by-value takes a copy at the time of the
// defer call rather than at the executation point of the defer.
func (rrl *RRL) incrementDebitStats(act *Action, ipr *IPReason, rtr *RTReason, ac AllowanceCategory) {
	rrl.statsMu.Lock()
	rrl.stats.incrementDebit(*act, *ipr, *rtr, ac)
	rrl.statsMu.Unlock()
}

func (rrl *RRL) incrementEviction() {
	rrl.statsMu.Lock()
	rrl.stats.Evictions++
	rrl.statsMu.Unlock()
}

// GetStats returns the internal stats accumulated by the Debit call.
// The caller can optionally request that the stats be zeroed after the copy.
func (rrl *RRL) GetStats(zeroAfter bool) (c Stats) {
	rrl.statsMu.Lock()
	c = rrl.stats.Copy(zeroAfter)
	rrl.statsMu.Unlock()
	c.CacheLength = rrl.table.Len()

	return
}
