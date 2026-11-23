package application

import (
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
)

func TestPutRejectsTemplateOwnedInstanceBeforeResolveOrPersist(t *testing.T) {
	resolver := &recordingResolver{}
	repository := &recordingRepository{}
	service := NewService(resolver, repository)
	instance := validInstance()
	instance.Source = domain.SourceTemplate

	if _, _, err := service.Put(instance, 0); !errors.Is(err, ErrTemplateOwnedInstance) {
		t.Fatalf("Put() error = %v, want ErrTemplateOwnedInstance", err)
	}
	if resolver.calls != 0 || repository.puts != 0 {
		t.Fatalf("Put() called resolver %d times and repository %d times", resolver.calls, repository.puts)
	}
}

func TestSetEnabledChecksRevisionBeforePersisting(t *testing.T) {
	instance := validInstance()
	instance.Revision = 4
	resolver := &recordingResolver{}
	repository := &recordingRepository{snapshot: Snapshot{Instances: []domain.Instance{instance}}}
	service := NewService(resolver, repository)

	if _, _, err := service.SetEnabled(instance.ID, false, 3); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("SetEnabled() error = %v, want ErrRevisionConflict", err)
	}
	if repository.puts != 0 {
		t.Fatalf("SetEnabled() persisted %d writes, want none", repository.puts)
	}
}

func validInstance() domain.Instance {
	return domain.Instance{ID: "redis-main", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Source: domain.SourceLocal}
}

type recordingResolver struct{ calls int }

func (r *recordingResolver) Resolve(instance domain.Instance) (domain.Instance, error) {
	r.calls++
	return instance, nil
}

type recordingRepository struct {
	snapshot Snapshot
	puts     int
}

func (r *recordingRepository) Put(instance domain.Instance, expected uint64) (domain.Instance, uint64, error) {
	r.puts++
	instance.Revision = expected + 1
	return instance, instance.Revision, nil
}
func (r *recordingRepository) Delete(string, uint64) (uint64, error) { return 0, nil }
func (r *recordingRepository) Snapshot() (Snapshot, error)           { return r.snapshot, nil }
