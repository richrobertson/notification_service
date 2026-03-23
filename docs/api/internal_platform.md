# github.com/richrobertson/notification-platform/internal/platform

_Generated from Go doc comments. Run `make docs-godoc` to refresh._

```text
package platform // import "github.com/richrobertson/notification-platform/internal/platform"

Package platform contains shared runtime helpers such as structured logging and
OpenTelemetry setup.

The package keeps process bootstrapping consistent across the API and worker
commands so startup, shutdown, and telemetry behavior do not drift between
binaries.

FUNCTIONS

func NewLogger(level string) *slog.Logger
    NewLogger builds the shared structured JSON logger used by the API and
    worker commands.

func SetupTelemetry(ctx context.Context, cfg config.Config) (func(context.Context) error, error)
    SetupTelemetry configures OpenTelemetry tracing and metrics for a process
    and returns a shutdown function that should be called during graceful
    shutdown.

```
