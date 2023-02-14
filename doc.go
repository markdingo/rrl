/*
Package rrl is a stand-alone implementation of ``Response Rate Limiting'' which helps
protect authoritative DNS servers from being used as a vehicle for amplification
attacks.
In addition to ``Response Rate Limiting'', rrl provides a configurable source address rate
limiter.

The rrl package is designed to be very easy to use.
It consists of a configuration mechanism and a single public function to check limits.
That's it; that's the interface.

``Response Rate Limiting`` was original devised by [ISC] and this implementation is
heavily derived from [COREDNSRRL] which mimics the ISC algorithms.

# Usage

The general pattern of use is to create a one-time [RRL] object with [NewRRL] using a
deployment-specific [Config], then call [Debit] prior to sending each response back to a
client.
[Debit] returns one of the following recommended actions: ``Send'', ``Drop'' or ``Slip''.

While the meaning of ``Send'' and ``Drop'' are self-evident, ``Slip'' is more complicated.

``Slip'' is ISC terminology which means to respond with a BADCOOKIE response or a
truncated response depending on whether the query included a valid client cookie or not.
The goal of a ``Slip'' response is to give genuine clients a small chance of getting a
response even when their source addresses are in a range being used as part of an
amplification attack.

The ``Slip'' response is one of a number of differences between a regular rate-limiting
system and the DNS-specific rrl.

Note that requests with valid server cookies are never rate-limited so a BADCOOKIE
response is always valid in the presence of a client cookie.

# Sample Code

The follow example demonstrates the expected pattern of use.
It introduces terms such as ``Response Tuple'', ``account'' and ``debit'' which are
explained in subsequent sections.
For now, the logic flow is of most relevance.

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

          action := rrl.Send                         // Default to sending response as-is
          if !request.validServerCook() {            // Only rate-limit if src can be spoofed
              tuple := makeTuple(response)           // Formulate the "Response Tuple"...
              action, _, _ := R.Debit(srcIP, tuple)  // ... and debit the corresponding accounts
          }

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
truncated responses thus the ``makeTruncatedIfAble'' function needs some intelligence.

# Concurrency

The rrl package is safe for concurrent use by multiple goroutines.
Normally a single [RRL] object is shared amongst all goroutines across the application.
However, if an application does require multiple [RRL] instances, they all operate
completely independently of each other.

# Background

While rate limiting is a common strategy used to limit abusive traffic, ``Response Rate
Limiting'' is specifically designed for UDP DNS queries (which lack a valid server cookie)
received by authoritative DNS servers.
The original RRL design was promulgated by [ISC] who have published extensive articles on
the subject.
A good place to start is their [ISCINTRO] document.

# Description

[RRL] tracks the query-per-second rate in a unique ``account'' assigned to each
``Response Tuple`` destined for a particular Client Network.

Each ``account'' is credited once per second by the configured amount and debited once for
each [Debit] call.
At most an ``account'' can gain up to one second of credits or up a configurable 15
seconds of debits.
While the ``account'' is in credit, a call to [Debit] returns a ``Send'' action.  If the
``account'' is not in credit, then a ``Drop'' or ``Slip'' action is returned depending on
the configured slip ratio.

## Response Tuple

A ``Response Tuple'' is an ``account'' key formulated from various features of the
response message depending on the nature of the response (NXDomain, Error, referral, etc).

The intent is for responses indicative of potential abuse to be assigned to a small set of
tuples whereas responses indicative of genuine requests are assigned to a large set of
tuples.
The goal being to cause ``accounts'' of suspect queries to run out of credits far sooner
than the ``accounts'' of genuine queries.

The formulation of a ``Response Tuple'' is somewhat convoluted.
Suffice to say that it varies considerably depending on the nature of the response - a
unique feature or rrl.
The [ResponseTuple] struct describes this formulation in detail.

## Client Network

The Client Network forms the other ``account'' key.
It is the source address of the request masked by the configured network size.
The default sizes being 24 for ipv4 and 56 for ipv6.

# Genesis

This package is derived from [COREDNSRRL] with the main differences being that all coredns
dependencies and external interfaces have been removed so that this package can be used by
standalone DNS implementations unrelated to coredns.

It goes without saying that this package only exists because of the efforts of the
coredns/rrl authors.
A big ``thank you'' to them.

The plan is for this project to mirror fixes and improvements to [COREDNSRRL] where
possible.

# References

[COREDNSRRL]: https://github.com/coredns/rrl
[ISC]: https://www.isc.org
[ISCINTRO]: https://kb.isc.org/docs/aa-01000

*/
package rrl
