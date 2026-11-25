// Package logger builds the process-wide structured logger (log/slog).
//
// Only adapters and platform code log. The domain and application layers
// return errors and let the edge decide what to record.
package logger
