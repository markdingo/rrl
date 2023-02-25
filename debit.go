package rrl

import (
	"net"
	"strings"
)

// An AllowanceCategory is the distillation of the rcode and response message the caller
// plans to send in response to a DNS query.
// Each category is associated with a separately configurable allowance used to decrement
// the rate-limiting account.
//
// The following table represents all categories and the selection rules which are
// evaluated in order from top to bottom with AllowanceError being the default if no other
// rules apply.
//
//	  AllowanceCategory  rCode   len(Answers)   len(Ns)
//	+-------------------+------+--------------+---------+
//	| AllowanceAnswer   |    0 |           >0 |         |
//	| AllowanceReferral |    0 |            0 |      >0 |
//	| AllowanceNoData   |    0 |            0 |       0 |
//	| AllowanceNXDomain |    3 |              |         |
//	| AllowanceError    |      |              |         |
//	+-------------------+------+--------------+---------+
//
// This table shows the configuration name associated with each AllowanceCategory.
//
//	  AllowanceCategory   Configuration Name
//	+-------------------+----------------------+
//	| AllowanceAnswer   | responses-per-second |
//	| AllowanceReferral | referrals-per-second |
//	| AllowanceNoData   | nodata-per-second    |
//	| AllowanceNXDomain | nxdomains-per-second |
//	| AllowanceError    | errors-per-second    |
//	+-------------------+----------------------+
type AllowanceCategory uint8

const (
	AllowanceAnswer AllowanceCategory = iota
	AllowanceReferral
	AllowanceNoData
	AllowanceNXDomain
	AllowanceError
	AllowanceLast
)

// NewAllowanceCategory is a helper function which creates an AllowanceCategory
func NewAllowanceCategory(rCode, answerCount, nsCount int) AllowanceCategory {
	switch {
	case rCode == 0 && answerCount > 0:
		return AllowanceAnswer
	case rCode == 0 && nsCount > 0:
		return AllowanceReferral
	case rCode == 0 && answerCount == 0:
		return AllowanceNoData
	case rCode == 3:
		return AllowanceNXDomain
	}

	return AllowanceError
}

// Action is the resulting recommendation returned by [Debit].
// Callers should act accordingly.
//
// Values are: Send, Drop and Slip (aka send truncated if able or BADCOOKIE response)
type Action int

const (
	Send Action = iota // Send the planned response
	Drop               // Do not send the planned response
	Slip               // Send a truncated response (if able) or a BADCOOKIE error
	ActionLast
)

// IPReason represents the state of IP rate limiting at the time the Action was
// determined.
// It is intended for diagnostic and statistical purposes only.
// Callers should expect that the range of reasons may increase or change over time.
//
// Values are: IPOk, IPNotConfigured, IPRateLimit and IPCacheFull.
type IPReason int

const (
	IPOk            IPReason = iota // IP CIDR is within rate limits
	IPNotConfigured                 // Config entry is zero
	IPNotReached                    // Not possible at this stage, but allow for possibility
	IPRateLimit                     // Ran out of credits
	IPCacheFull                     // RRL cache failed to create a new account
	IPLast
)

// RTReason represents the state of "Response Tuple" rate limiting at the time the Action
// was determined.
// It is intended for diagnostic and statistical purposes only.
// Callers should expect that the range of reasons may increase or change over time.
//
// Values are: RTOk, RTNotConfigured, RTNotReached, RTRateLimit, RTNotUDP and RTCacheFull.
type RTReason int

const (
	RTOk            RTReason = iota // Account is in credit
	RTNotConfigured                 // Config entry is zero
	RTNotReached                    // An earlier condition causes Action (IPLimit most likely)
	RTRateLimit                     // Ran out of credits
	RTNotUDP                        // Debit is only applicable to UDP queries
	RTCacheFull                     // RRL cache failed to create a new account
	RTLast
)

// ResponseTuple is provided by the application when calling [Debit]. It is used
// internally as a "database key" to uniquely identify rate-limiting "accounts".
// [Debit] expects all fields to be filled - with the exception noted below.
//
// To fully populate a ResponseTuple the caller needs access to the response message and
// whether the answer was formulated dynamically by such things as wildcards or synthetic
// answers (as often used in reverse serving).
// When dynamically generated the caller needs to know the origin name of the dynamically
// created resource.
//
// Fields are:
//
//   - Class - The class of the query which is highly likely to be ClassINET. This value is
//     a direct copy of the numeric value in the DNS question RR.
//
//   - Type - The type of the query such as TypeA, TypeNS, etc. This valus is a
//     direct copy of the numeric value in the DNS Question RR.
//
//   - [AllowanceCategory] is derived from the rtype and RR counts in the response message.
//     It effectively collapses a wide range of rtypes and response types down to a small
//     subset which are of most interest to rrl.
//
//     Values are: AllowanceAnswer, AllowanceReferral, AllowanceNoData, AllowanceNXDomain and the catchall
//     AllowanceError when none of the other AllowanceCategorys apply.
//     The [AllowanceCategory] type documents the rules for setting these values.
//
//   - SalientName the name to use for the purpose of uniquely identifying the query.
//     In the simplest case it is a copy of the qName from the first RR in the Question section of
//     the response, but it varies according to the selection rules.
//
// ### SalientName Selection Rules
//
// These rules must be evaluated in sequential order.
//
//  1. If AllowanceCategory is AllowanceNXDomain or AllowanceReferral then use the qName in the first
//     RR in the Ns section of the response.
//     If the Ns section is empty, set SalientName to an empty string.
//
//  2. If the response is dynamically synthesized - perhaps from a wildcard - set
//     SalientName to the origin name prefixed with "*". E.g. "*.example.com".
//
//     The goal is to group all dynamic responses under the one "account" as otherwise the
//     potentially huge range of responses are distributed across an equally huge number
//     of rate-limiting "accounts" which largely defeats the purpose of rrl.
//
//     Quite often the origin name is simply all labels to the right of the leading
//     dynamic label, but be aware that the origin name may be further up the delegation
//     tree.
//     The caller must be able to determine the actual origin name to reliably make this
//     determination.
//     By way of example, the following zone entry:
//
//     *.*.*.example.com IN TXT "Hello Worms"
//
//     should result in a SalientName of "example.com".
//
//  3. If neither of the previous conditions apply set SalientName to the qName from the
//     first RR in the Question section of the response.
//
// In the very unlikely event that the response message only contains a COOKIE OPT as
// allowed in RFC7873#5.4, none of the ResponseTuple fields should be populated except
// [AllowanceCategory].
type ResponseTuple struct {
	Class uint16
	Type  uint16
	AllowanceCategory
	SalientName string
}

// Debit decrements the "account" associated with the Client Network and "Response Tuple".
// It returns a recommended action and reasons for recommending that action.
//
// Debit should only be called for queries which do not contain a valid server
// cookie. Since Debit cannot check for a valid server cookie - the caller is responsible
// for this part.
//
// src is the purported source address of the client who sent the query - this is
// masked by the configured network prefix lengths to determine the Client Network.
//
// tuple is the [ResponseTuple] formulated from the response and related information (in
// particular whether the response was formulated from a wildcard).
//
// ## Returned Values
//
// [Action] indicates what the caller should do with the response as a consequence of RRL
// processing - it can be one of Send, Drop or Slip.
//
// [IPReason] and [RTReason] provide insights as to why the action was recommended.
// They may be useful details for statistics and logging purposes.
//
// Debit is concurrency safe.
func (rrl *RRL) Debit(src net.Addr, tuple *ResponseTuple) (act Action, ipr IPReason, rtr RTReason) {
	act = Send
	ipr = IPNotConfigured
	rtr = RTNotReached

	// Must use pointers to return values as otherwise defer takes a copy of the
	// values at the defer call site, which is as they are now rather than at the end
	// of the function. This is common knowledge, but easily forgotten.

	defer rrl.incrementDebitStats(&act, &ipr, &rtr, tuple.AllowanceCategory)

	ipPrefix := rrl.addrPrefix(src.String()) // Need this for both rate limiting tests

	// Rate limit on a source-address basis regardless of whether it's TCP or UDP
	if rrl.cfg.requestsInterval != 0 {
		b, _, err := rrl.debit(rrl.cfg.requestsInterval, ipPrefix) // ignore slip for IP limits
		if err != nil {
			act = Drop
			ipr = IPCacheFull
			return
		}
		// if the balance is negative, drop the request (don't write response to client)
		if b < 0 {
			act = Drop
			ipr = IPRateLimit
			return
		}
		ipr = IPOk
	}

	// RRL on query only applies to udp. All other transports are assumed to be
	// resistant to source address spoofing. Filter on all types of udp, such as udp,
	// udp4 & udp6.
	if !strings.HasPrefix(src.Network(), "udp") {
		rtr = RTNotUDP
		return
	}

	allowance := rrl.allowanceForRtype(tuple.AllowanceCategory) // What is the configured cost for this query type?
	if allowance == 0 {
		rtr = RTNotConfigured
		return
	}

	// Insulate against unbound/use-caps-for-id et al when generating cache key
	name := strings.ToLower(tuple.SalientName)
	t := rrl.accountToken(ipPrefix, tuple.Type, name, tuple.AllowanceCategory)

	// Debit account and get results
	b, slip, err := rrl.debit(allowance, t)
	if err != nil {
		act = Drop
		rtr = RTCacheFull
		return
	}

	// If the balance is negative, rate limit the response
	if b < 0 {
		rtr = RTRateLimit
		if slip {
			act = Slip
		} else {
			act = Drop
		}
		return
	}

	rtr = RTOk // Yeah, we're all good to go

	return
}
