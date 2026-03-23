// Package delivery contains the delivery-time execution logic for channel
// workers.
//
// It sits between queue jobs and durable store updates:
//
//   - queue.DispatchJob identifies the notification attempt to process
//   - store.Postgres provides durable attempt and dead-letter state changes
//   - webhook and SMTP senders perform the external side effects
//
// The package is deliberately honest about its guarantees. Delivery is still
// at-least-once underneath, but the service layers attempt-state guards,
// duplicate suppression, retry scheduling, dead-lettering, and audited failover
// behavior on top.
//
// New contributors should read Service first. It is the main orchestration
// entry point and shows the end-to-end processing contract that workers rely
// on.
package delivery
