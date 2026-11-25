// Package config loads the process configuration from environment variables.
//
// Every value is read once at startup, validated, and returned as a typed
// struct. Nothing else in the codebase reads the environment, so the full set
// of settings is visible in one place and testable without touching os.Getenv.
package config
