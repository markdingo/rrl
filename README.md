<!-- Always newline after period so diffs are easier to read. -->
# rrl - Response Rate Limiting for DNS Servers

## Introduction

`rrl` is a standalone `go` package which implements the [ISC](https://www.isc.org) [Response
Rate Limiting](https://kb.isc.org/docs/aa-01148) algorithms as originally implemented in
[Bind 9](https://www.isc.org/bind/).
The goal of "Response Rate Limiting" is to help authoritative DNS servers mitigate against
being used as part of an amplification attack.
Such attacks are very easy to orchestrate since most authoritative DNS servers respond to
UDP queries from any putative source address.

If you are the developer of an authoritative DNS server then in the interest of Internet
hygiene you really should incorporate a "Response Rate Limiting" capability - whether with
this package or some other.
If you don't, your server is more vulnerable to being used as part of an amplification
attack which will not be regarded highly by other DNS operators.

This package is designed to be very easy to use.
It consists of a configuration mechanism and a single public function to check limits.
That's it; that's the interface.

If you use [miekg/dns](https://github.com/miekg/dns) you might
find it convenient to use [markdingo/miekgrrl](https://github.com/markdingo/miekgrrl) which
provides an adaptor function for passing `miekg.dns.Msg` attributes to this package.

## Genesis

This package is derived from [coredns/rrl](https://github.com/coredns/rrl) which mimics
the ISC algorithms.

The main differences between this package and coredns/rrl is that all coredns dependencies
and external interfaces have been removed so that this package can be used by programs
unrelated to coredns.
For example the external logging and statistics functions have been removed and are now
the responsibility of the caller.
In short, all external interactions and dependencies have been eliminated, but otherwise the underlying
implementation is largely unchanged.

(Needless to say, this package only exists because of the efforts of the coredns/rrl developers.
A big "thank you" to them.)

### Project Status

[![Build Status](https://github.com/markdingo/rrl/actions/workflows/go.yml/badge.svg)](https://github.com/markdingo/rrl/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/markdingo/rrl/branch/main/graph/badge.svg)](https://codecov.io/gh/markdingo/rrl)
[![CodeQL](https://github.com/markdingo/rrl/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/markdingo/rrl/actions/workflows/codeql-analysis.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/markdingo/rrl)](https://goreportcard.com/report/github.com/markdingo/rrl)
[![Go Reference](https://pkg.go.dev/badge/github.com/markdingo/rrl.svg)](https://pkg.go.dev/github.com/markdingo/rrl)

## Description

`rrl` is called by an authoritative DNS server prior to sending each response to a query.
`rrl` tracks the query-per-second rate in a unique "account" assigned to each "Response
Tuple" destined for a particular Client Network.

"Accounts" are credited each second with a configured amount and debited once for
each call to [Debit].
At most an "account" can gain up to one second of credits or up to a configurable 15
seconds of debits.
While the "account" is in credit `rrl` indicates that the caller should send their planned response.
Otherwise `rrl` indicates that the caller should `Drop` or `'Slip'` their response.

The "Response Tuple" is formulated from the most salient features of the response message.
This formulation is somewhat convoluted because DNS responses are somewhat convoluted.

`'Slip'` is ISC terminology which means to respond with a BADCOOKIE response or a
truncated response depending on whether the query contained a valid client cookie or not.
The goal of a `'Slip'` response is to give genuine clients a small chance of getting a
response even when their source addresses are in a range being used as part of an
amplification attack.

`rrl` plays no part in processing DNS messages or modifying them for output - it solely
tracks rate-limiting "accounts" and returns a recommended course of action.
All DNS actions, statistics gathering and logging are the responsibility of the caller.

### "Response Tuple" and Client Network

"Response Tuple" and Client Network are used to uniquely identity rate-limiting
"accounts".
In effect they form keys to an internal `rrl` "accounts" cache.

A "Response Tuple" is formulated from various features of the response message - the exact
details depend on the nature of the response (NXDomain, Error, referral, etc).
To paraphrase ISC, the formulation of the "Response Tuple" is not simplistic.
The intent is for responses indicative of potential abuse to be assigned to a small set of
tuples whereas responses indicative of genuine requests are assigned to a large set of
tuples.
The goal being to cause "accounts" of suspect queries to run out of credits far sooner
than the "accounts" of genuine queries.

The package documentation describes how to formulate a "Response Tuple".

A Client Network is the putative source address of the request masked by the configured
size of the "network".
The default configured sizes being 24 for ipv4 and 56 for ipv6.

### ISC Terminology

As a general rule, this documentation uses ISC terminology, such as "accounts" and
"debits" and so on.
The one exception being "Response Tuple" which is used in preference to "Identical
Response", or "Token" in coredns/rrl parlance.
While there is obvious merit in common terminology, "Response Tuple" seem to better
convey intent and outcome.

## Sample Code

    package main

    import "github.com/markdingo/rrl"

    func main() {

      server:= dnsListenSocket()
      db := myDatabase()
          
      cfg := rrl.NewConfig()
      cfg.SetValue(...)             // Configure limits relevant to our deployment
      R := NewRRL(cfg)              // Create our `rrl` instance

      for {
          srcIP, request := server.GetRequest()      // Accept a query
          response := db.lookupResponse(request)     // Create the response

          tuple := makeTuple(response)               // Formulate the "Response Tuple"...
          action, _, _ := R.Debit(srcIP, tuple)      // ... and debit the corresponding accounts

          switch action {                            // Dispatch on the recommended action

          case rrl.Drop:                             // Drop is easy, do nothing

          case rrl.Send:                             // No rate limit applies, ship it!
              server.Send(response)

          case rrl.Slip:
              if request.ValidClientCookie() {       // Slip response varies depending on
                  server.SendBadCookie(response)     // whether the client sent a cooke or not
              } else {
                  response.makeTruncatedIfAble()     // No valid client cookie means
                  server.Send(response)              // send a truncated response
              }
          }
      }
    }

Note that some error responses such as REFUSED and SERVFAIL cannot be replaced with
truncated responses thus the `makeTruncatedIfAble` function needs some intelligence.

## Installation

`rrl` requires [go](https://golang.org) version 1.19 or later.

Once your application imports `"github.com/markdingo/rrl"`, then `"go build"` or `"go mod
tidy"` should download and compile the `rrl` package automatically.

## Further Reading

The `rrl` API is described in the package documentation which is mirrored online at
[pkg.go.dev](https://pkg.go.dev/github.com/markdingo/rrl).
Other background material can be found at the [coredns rrl
plugin](https://github.com/coredns/rrl) home page.

## Community

If you have any problems using `rrl` or suggestions on how it can do a better job,
don't hesitate to create an [issue](https://github.com/markdingo/rrl/issues) on
the project home page.
This package can only improve with your feedback.

## Motivation

`rrl` was originally created for [autoreverse](https://github.com/markdingo/autoreverse),
the no muss, no fuss reverse DNS server; check
[it](https://github.com/markdingo/autoreverse) out if you want an example of how `rrl` is
used in the wild.

## Copyright and License

`rrl` is Copyright :copyright: 2023 Mark Delany and is licensed under the BSD
2-Clause "Simplified" License.
