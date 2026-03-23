// Package pressure tracks overload signals used by the API, retry logic, and
// workers.
//
// It combines queue-depth snapshots with in-process counters so callers can
// answer two different questions:
//
//   - What is Redis reporting right now?
//   - How often has the service recently rejected, throttled, or saturated?
//
// The resulting metrics are intentionally lightweight and operationally useful,
// not a full-blown policy engine.
package pressure
