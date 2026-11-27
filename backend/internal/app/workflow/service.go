package workflow

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// presetsTTL is how long presets stay cached. A published version never
// changes, so the TTL only bounds how soon a newly published version, or a
// moved default, shows up (backend/CLAUDE.md, Redis).
const presetsTTL = 10 * time.Minute

// Service implements UseCases and Source over a Repository and the cache.
type Service struct {
	repo  Repository
	tx    appshared.TxManager
	cache appshared.Cache
}

var (
	_ UseCases = (*Service)(nil)
	_ Source   = (*Service)(nil)
)

// NewService returns a Service. tx runs the reads: the repository needs a
// transaction even for global tables, and a read inside the caller's own
// transaction joins it.
func NewService(repo Repository, tx appshared.TxManager, cache appshared.Cache) *Service {
	return &Service{repo: repo, tx: tx, cache: cache}
}

func presetsKey() (appshared.Key, error) { return appshared.GlobalKey("workflow", "presets") }

func versionKey(id shared.WorkflowID, version int) (appshared.Key, error) {
	return appshared.GlobalKey("workflow", id.String(), strconv.Itoa(version))
}

// Presets returns the latest version of every preset, sorted by key, cached
// under g:workflow:presets for 10 minutes. Any role may call it
// (settings:read). A cache that fails is a miss.
func (s *Service) Presets(ctx context.Context) ([]View, error) {
	if _, err := authz.Require(ctx, identity.PermSettingsRead); err != nil {
		return nil, err
	}
	key, err := presetsKey()
	if err != nil {
		return nil, err
	}
	var cached presetList
	// A cache error is a miss: the adapter logs it and Postgres answers.
	if hit, err := s.cache.Get(ctx, key, &cached); err == nil && hit {
		return cached.Items, nil
	}
	var ws []workflow.Workflow
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		read, err := s.repo.Published(ctx)
		if err != nil {
			return fmt.Errorf("list published workflows: %w", err)
		}
		ws = read
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := presetList{Items: make([]View, 0, len(ws))}
	for _, w := range ws {
		out.Items = append(out.Items, viewOf(w))
	}
	_ = s.cache.Set(ctx, key, out, presetsTTL) // a failed Set only costs the next read
	return out.Items, nil
}

// Get returns one published version, cached under g:workflow:{id}:{version}
// for 10 minutes. It returns an error matching shared.ErrNotFound for a
// version that is not published, and never caches that answer, so a version
// published later is found at once.
func (s *Service) Get(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error) {
	key, err := versionKey(id, version)
	if err != nil {
		return workflow.Workflow{}, fmt.Errorf("workflow %s v%d: %w", id, version, shared.ErrNotFound)
	}
	var cached View
	if hit, err := s.cache.Get(ctx, key, &cached); err == nil && hit {
		if w, err := cached.workflowOf(); err == nil {
			return w, nil
		}
		// A cached entry that no longer builds is a miss.
	}
	var w workflow.Workflow
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		read, err := s.repo.ByVersion(ctx, id, version)
		if err != nil {
			return fmt.Errorf("read workflow %s v%d: %w", id, version, err)
		}
		w = read
		return nil
	})
	if err != nil {
		return workflow.Workflow{}, err
	}
	_ = s.cache.Set(ctx, key, viewOf(w), presetsTTL)
	return w, nil
}

// Default returns the preset new stores start with. It is read from
// Postgres every time: only tenant creation asks, and a moved default must
// show at once.
func (s *Service) Default(ctx context.Context) (workflow.Workflow, error) {
	var w workflow.Workflow
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		read, err := s.repo.Default(ctx)
		if err != nil {
			return fmt.Errorf("read default workflow: %w", err)
		}
		w = read
		return nil
	})
	return w, err
}
