// Package worker contains the queue-consuming runtime loops used by the
// dispatcher and channel workers.
//
// The package focuses on practical execution concerns:
//
//   - fair per-tenant scheduling
//   - bounded worker concurrency
//   - reserve/process/ack behavior
//   - recovery of stranded processing queues
//   - graceful shutdown that drains in-flight work
//
// It does not own delivery semantics itself; instead it coordinates queue
// mechanics around a caller-supplied Processor function.
package worker
