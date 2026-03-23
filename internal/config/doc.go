// Package config loads runtime configuration from environment variables and
// validates that the process can start safely.
//
// The package is intentionally small and explicit:
//
//   - Load reads environment variables and applies local-development defaults.
//   - Validate checks cross-field invariants such as retry windows, queue limits,
//     request size limits, and endpoint formatting.
//   - ValidateForAPI adds the API-specific admin-token requirement.
//
// Experienced maintainers can treat this package as the single reference for
// supported environment variables. New contributors should start here before
// adding new runtime knobs so the startup behavior, defaults, and validation
// rules stay aligned.
package config
