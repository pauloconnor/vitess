/*
Copyright 2017 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreedto in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package buffer

import "vitess.io/vitess/go/stats"

// This file contains all status variables which can be used to monitor the
// buffer.

var (
	// starts counts how often we started buffering (including dry-run bufferings).
	starts = stats.NewMultiCounters(
		"BufferStarts",
		"How many times we started buffering, including dry-run",
		[]string{"Keyspace", "ShardName"})
	// stops counts how often we triggered the stop of a buffering, including
	// dry-run bufferings.
	// See the type "stopReason" below for all possible values of "Reason".
	stops = stats.NewMultiCounters(
		"BufferStops",
		"How many times we triggered a buffering to stop, including dry-runs",
		[]string{"Keyspace", "ShardName", "Reason"})

	// failoverDurationSumMs is the cumulative sum of all failover durations.
	// In connection with "starts" it can be used to calculate a moving average.
	failoverDurationSumMs = stats.NewMultiCounters(
		"BufferFailoverDurationSumMs",
		"Cumulative sum of all failover durations",
		[]string{"Keyspace", "ShardName"})

	// utilizationSum is the cumulative sum of the maximum buffer utilization
	// (in percentage) during each failover.
	// Utilization = maximum number of requests buffered / buffer size.
	// In connection with "starts" it can be used to calculate a moving average.
	// TODO(mberlin): Replace this with a MultiHistogram once it's available.
	utilizationSum = stats.NewMultiCounters(
		"BufferUtilizationSum",
		"Cumulative sum of the max buffer utilization (in %) during each failover",
		[]string{"Keyspace", "ShardName"})
	// utilizationDryRunSum is the cumulative sum of the maximum *theoretical*
	// buffer utilization (in percentage) during each failover.
	// Utilization = maximum number of requests buffered seen / buffer size.
	// In connection with "starts" it can be used to calculate a moving average.
	// Example: Buffer size = 10. Two consecutive failovers with maximum values of
	// 15 and 5 seen requests each add up to a value of 200% (150% + 50%
	// utilization). The moving average would be 100% because there were two
	// failovers in that period.
	// TODO(mberlin): Replace this with a MultiHistogram once it's available.
	utilizationDryRunSum = stats.NewMultiCounters(
		"BufferUtilizationDryRunSum",
		"Cumulative sum of the max *theoretical* buffer util (in %) during each failover",
		[]string{"Keyspace", "ShardName"})

	// requestsBuffered tracks how many requests were added to the buffer.
	// NOTE: The two counters "Buffered" and "Skipped" should cover all requests
	// which passed through the buffer.
	requestsBuffered = stats.NewMultiCounters(
		"BufferRequestsBuffered",
		"How many requests were added to the buffer",
		[]string{"Keyspace", "ShardName"})
	// requestsBufferedDryRun tracks how many requests would have been added to
	// the buffer (dry-run mode).
	requestsBufferedDryRun = stats.NewMultiCounters(
		"BufferRequestsBufferedDryRun",
		"How many requests would've been added to the buffer (dry-run)",
		[]string{"Keyspace", "ShardName"})
	// requestsBuffered tracks how many requests were drained from the buffer.
	// NOTE: The sum of the two counters "Drained" and "Evicted" should be
	// identical to the "Buffered" counter value.
	requestsDrained = stats.NewMultiCounters(
		"BufferRequestsDrained",
		"How many requests drained from the buffer",
		[]string{"Keyspace", "ShardName"})
	// requestsEvicted tracks how many requests were evicted early from the buffer.
	// See the type "evictedReason" below for all possible values of "Reason".
	requestsEvicted = stats.NewMultiCounters(
		"BufferRequestsEvicted",
		"How many requests were evicted early from the buffer",
		[]string{"Keyspace", "ShardName", "Reason"})
	// requestsSkipped tracks how many requests would have been buffered but
	// eventually were not (includes dry-run bufferings).
	// See the type "skippedReason" below for all possible values of "Reason".
	requestsSkipped = stats.NewMultiCounters(
		"BufferRequestsSkipped",
		"How many requests would've been buffered but were skipped (incl. dry-run)",
		[]string{"Keyspace", "ShardName", "Reason"})
)

// stopReason is used in "stopsByReason" as "Reason" label.
type stopReason string

var stopReasons = []stopReason{stopFailoverEndDetected, stopMaxFailoverDurationExceeded, stopShutdown}

const (
	stopFailoverEndDetected         stopReason = "NewMasterSeen"
	stopMaxFailoverDurationExceeded            = "MaxDurationExceeded"
	stopShutdown                               = "Shutdown"
)

// evictedReason is used in "requestsEvicted" as "Reason" label.
type evictedReason string

var evictReasons = []evictedReason{evictedContextDone, evictedBufferFull, evictedWindowExceeded}

const (
	evictedContextDone    evictedReason = "ContextDone"
	evictedBufferFull                   = "BufferFull"
	evictedWindowExceeded               = "WindowExceeded"
)

// skippedReason is used in "requestsSkipped" as "Reason" label.
type skippedReason string

var skippedReasons = []skippedReason{skippedBufferFull, skippedDisabled, skippedShutdown, skippedLastReparentTooRecent, skippedLastFailoverTooRecent}

const (
	// skippedBufferFull occurs when all slots in the buffer are occupied by one
	// or more concurrent failovers. Unlike "evictedBufferFull", no request could
	// be evicted and therefore we had to skip this request.
	skippedBufferFull skippedReason = "BufferFull"
	// skippedDisabled is used when the buffer was disabled for that particular
	// keyspace/shard.
	skippedDisabled              = "Disabled"
	skippedShutdown              = "Shutdown"
	skippedLastReparentTooRecent = "LastReparentTooRecent"
	skippedLastFailoverTooRecent = "LastFailoverTooRecent"
)

// initVariablesForShard is used to initialize all shard variables to 0.
// If we don't do this, monitoring frameworks may not correctly calculate rates
// for the first failover of the shard because they see a transition from
// "no value for this label set (NaN)" to "a value".
// "statsKey" should have two members for keyspace and shard.
func initVariablesForShard(statsKey []string) {
	starts.Set(statsKey, 0)
	for _, reason := range stopReasons {
		key := append(statsKey, string(reason))
		stops.Set(key, 0)
	}

	failoverDurationSumMs.Set(statsKey, 0)

	utilizationSum.Set(statsKey, 0)
	utilizationDryRunSum.Set(statsKey, 0)

	requestsBuffered.Set(statsKey, 0)
	requestsBufferedDryRun.Set(statsKey, 0)
	requestsDrained.Set(statsKey, 0)
	for _, reason := range evictReasons {
		key := append(statsKey, string(reason))
		requestsEvicted.Set(key, 0)
	}
	for _, reason := range skippedReasons {
		key := append(statsKey, string(reason))
		requestsSkipped.Set(key, 0)
	}
}

// TODO(mberlin): Remove the gauge values below once we store them
// internally and have a /bufferz page where we can show this.
var (
	// bufferSize publishes the configured per vtgate buffer size. It can be used
	// to calculate the utilization of the buffer.
	bufferSize = stats.NewInt("BufferSize")
	// lastFailoverDurationMs tracks for how long vtgate buffered requests during
	// the last failover.
	// The value for a given shard will be reset at the next failover.
	lastFailoverDurationMs = stats.NewMultiCounters(
		"BufferLastFailoverDurationMs",
		"How long vtgate buffered requests during the last failover",
		[]string{"Keyspace", "ShardName"})
	// lastRequestsInFlightMax has the maximum value of buffered requests in flight
	// of the last failover.
	// The value for a given shard will be reset at the next failover.
	lastRequestsInFlightMax = stats.NewMultiCounters(
		"BufferLastRequestsInFlightMax",
		"The max value of buffered requests in flight of the last failover",
		[]string{"Keyspace", "ShardName"})
	// lastRequestsDryRunMax has the maximum number of requests which were seen during
	// a dry-run buffering of the last failover.
	// The value for a given shard will be reset at the next failover.
	lastRequestsDryRunMax = stats.NewMultiCounters(
		"BufferLastRequestsDryRunMax",
		"Max # of requests which were seen during a dry-run buffering of the last failover",
		[]string{"Keyspace", "ShardName"})
)
