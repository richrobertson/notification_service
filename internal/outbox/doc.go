// Package outbox publishes durable dispatch intents from Postgres into Redis.
//
// The package is intentionally narrow: it is not a generalized event bus or a
// change-data-capture layer. Its only responsibility is to take dispatch work
// that has already been durably recorded in Postgres and enqueue it onto the
// Redis execution transport.
//
// RunOnce is the main entry point and is designed for simple polling workers.
package outbox
