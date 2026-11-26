package auth_test

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/sharedports"
)

func TestSecrets_PassesTheSecretGeneratorContract(t *testing.T) {
	t.Parallel()
	sharedports.RunSecretGeneratorContract(t, func(*testing.T) appshared.SecretGenerator { return auth.NewSecrets() })
}
