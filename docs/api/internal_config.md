# github.com/richrobertson/notification-platform/internal/config

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package config // import "github.com/richrobertson/notification-platform/internal/config"

Package config loads runtime configuration from environment variables and
validates that the process can start safely.

The package is intentionally small and explicit:

  - Load reads environment variables and applies local-development defaults.
  - Validate checks cross-field invariants such as retry windows, queue limits,
    request size limits, and endpoint formatting.
  - ValidateForAPI adds the API-specific admin-token requirement.

Experienced maintainers can treat this package as the single reference for
supported environment variables. New contributors should start here before
adding new runtime knobs so the startup behavior, defaults, and validation rules
stay aligned.

TYPES

type Config struct {
	AppName                         string
	HTTPPort                        string
	LogLevel                        string
	DatabaseURL                     string
	OTLPEndpoint                    string
	Environment                     string
	RedisAddr                       string
	RedisPassword                   string
	RedisDB                         int
	HTTPReadHeaderTimeout           time.Duration
	HTTPReadTimeout                 time.Duration
	HTTPWriteTimeout                time.Duration
	HTTPIdleTimeout                 time.Duration
	HTTPShutdownTimeout             time.Duration
	MaxRequestBodyBytes             int64
	AdminToken                      string
	WebhookTimeout                  time.Duration
	QueueBlockTimeout               time.Duration
	RetryMaxAttempts                int
	RetryBaseDelay                  time.Duration
	RetryMaxDelay                   time.Duration
	RetryExponentialBackoff         bool
	RetryJitter                     time.Duration
	RetryWorkerPollInterval         time.Duration
	OutboxPollInterval              time.Duration
	SchedulerPollInterval           time.Duration
	RecoveryInterval                time.Duration
	SMTPHost                        string
	SMTPPort                        int
	SMTPUsername                    string
	SMTPPassword                    string
	SMTPFrom                        string
	SMTPUseTLS                      bool
	SMTPStartTLS                    bool
	SMTPInsecureSkipVerify          bool
	SecondarySMTPHost               string
	SecondarySMTPPort               int
	SecondarySMTPUsername           string
	SecondarySMTPPassword           string
	SecondarySMTPFrom               string
	SecondarySMTPUseTLS             bool
	SecondarySMTPStartTLS           bool
	SecondarySMTPInsecureSkipVerify bool
	APIRateLimitPerSecond           int
	APIRateLimitWindow              time.Duration
	QueueSoftLimit                  int
	QueueHardLimit                  int
	BackpressureRetryAfter          time.Duration
	DispatcherConcurrency           int
	EmailWorkerConcurrency          int
	WebhookWorkerConcurrency        int
	PerTenantWorkerBurst            int
	PerTenantMaxInFlight            int
	RetryPressureMultiplier         int
	RetryPressureMinDelay           time.Duration
	MaintenanceAuditRetention       time.Duration
	MaintenanceOutboxRetention      time.Duration
	MaintenanceDeadLetterRetention  time.Duration
	MaintenanceDryRun               bool
}
    Config contains the process-level runtime settings used by the API and
    workers.

func Load() Config
    Load reads configuration from environment variables and applies pragmatic
    local-development defaults.

    The returned value is intentionally usable before validation so commands
    can tailor app names or command-specific behavior and then call Validate or
    ValidateForAPI.

func (c Config) Validate() error
    Validate checks that the configuration is internally consistent and safe to
    use for a long-running process.

    The method validates both individual fields and important cross-field
    invariants such as retry ranges, queue limits, and timeout ordering.

func (c Config) ValidateForAPI() error
    ValidateForAPI applies the shared validation rules and then enforces the
    additional operator-token requirement used by the HTTP API.

```
