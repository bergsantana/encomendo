package main

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// The composition root refuses the development signing key in production,
// as signing and as verify-only key, before it builds anything (ADR 0019).
// LoadKeys has its own tests; this one pins that main asks it to refuse.
func TestBuildServices_RefusesTheDevKeyInProduction(t *testing.T) {
	t.Parallel()

	own := "q0Fcl2nSQ0n6X3zW1T8Vq0Fcl2nSQ0n6X3zW1T8Vq0E="
	log := slog.New(slog.DiscardHandler)
	for name, a := range map[string]config.Auth{
		"as signing key":     {SigningSeed: auth.DevSigningSeed, Issuer: "https://api.encomendo.com.br"},
		"as verify-only key": {SigningSeed: own, VerifySeeds: []string{auth.DevSigningSeed}, Issuer: "https://api.encomendo.com.br"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := buildServices(config.Config{Env: config.EnvProduction, Auth: a}, log, wiring{})
			if err == nil || !strings.Contains(err.Error(), "development signing key") {
				t.Errorf("buildServices = %v, want the development key refused", err)
			}
		})
	}
}
