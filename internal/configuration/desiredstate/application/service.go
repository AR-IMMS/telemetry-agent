package application

import (
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
)

var (
	// ErrRevisionConflict reports an optimistic concurrency mismatch.
	ErrRevisionConflict = errors.New("integration revision conflict")
	// ErrInstanceNotFound identifies a desired Integration absent from the snapshot.
	ErrInstanceNotFound = errors.New("Integration not found")
	// ErrTemplateOwnedInstance prevents local CRUD from mutating generated ownership records.
	ErrTemplateOwnedInstance = errors.New("Integration instance is owned by a Template")
)

// Snapshot is one consistent desired Integration set and its change marker.
type Snapshot struct {
	Revision  uint64
	Instances []domain.Instance
}

// Repository persists desired instances using optimistic concurrency.
type Repository interface {
	Put(domain.Instance, uint64) (domain.Instance, uint64, error)
	Delete(string, uint64) (uint64, error)
	Snapshot() (Snapshot, error)
}

// Resolver validates an instance and applies Monitoring-owned definition rules.
// Its implementation is supplied at the composition boundary.
type Resolver interface {
	Resolve(domain.Instance) (domain.Instance, error)
}

// Service coordinates desired-state use cases through Monitoring and persistence ports.
type Service struct {
	resolver   Resolver
	repository Repository
}

// NewService wires a Monitoring resolver port to the desired-state repository.
func NewService(resolver Resolver, repository Repository) Service {
	return Service{resolver: resolver, repository: repository}
}

// Put validates and resolves an instance before applying an expected-revision mutation.
func (s Service) Put(instance domain.Instance, expected uint64) (domain.Instance, uint64, error) {
	if instance.Source == domain.SourceTemplate {
		return domain.Instance{}, 0, ErrTemplateOwnedInstance
	}
	resolved, err := s.resolver.Resolve(instance)
	if err != nil {
		return domain.Instance{}, 0, err
	}
	stored, revision, err := s.repository.Put(resolved, expected)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("store desired Integration: %w", err)
	}
	return stored, revision, nil
}

// SetEnabled changes only the desired lifecycle flag using the caller's expected instance revision.
func (s Service) SetEnabled(id string, enabled bool, expected uint64) (domain.Instance, uint64, error) {
	if s.repository == nil {
		return domain.Instance{}, 0, fmt.Errorf("Integration repository is required")
	}
	snapshot, err := s.repository.Snapshot()
	if err != nil {
		return domain.Instance{}, 0, err
	}
	for _, instance := range snapshot.Instances {
		if instance.ID != id {
			continue
		}
		if instance.Revision != expected {
			return domain.Instance{}, 0, ErrRevisionConflict
		}
		instance.Enabled = enabled
		return s.Put(instance, expected)
	}
	return domain.Instance{}, 0, ErrInstanceNotFound
}

// PutPreservingEnabled updates an instance while retaining its current enabled state.
func (s Service) PutPreservingEnabled(instance domain.Instance, expected uint64) (domain.Instance, uint64, error) {
	if s.repository == nil {
		return domain.Instance{}, 0, fmt.Errorf("Integration repository is required")
	}
	snapshot, err := s.repository.Snapshot()
	if err != nil {
		return domain.Instance{}, 0, err
	}
	for _, existing := range snapshot.Instances {
		if existing.ID != instance.ID {
			continue
		}
		if existing.Revision != expected {
			return domain.Instance{}, 0, ErrRevisionConflict
		}
		instance.Enabled = existing.Enabled
		return s.Put(instance, expected)
	}
	return domain.Instance{}, 0, ErrInstanceNotFound
}
