// Package platform contains shared runtime helpers such as structured logging
// and OpenTelemetry setup.
//
// The package keeps process bootstrapping consistent across the API and worker
// commands so startup, shutdown, and telemetry behavior do not drift between
// binaries.
package platform
