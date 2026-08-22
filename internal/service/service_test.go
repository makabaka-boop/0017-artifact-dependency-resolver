package service

import (
	"strings"
	"testing"
	"time"

	"artifact-dep-resolver/internal/model"
	"artifact-dep-resolver/internal/store"
)

func TestValidArtifactNameDefersToModel(t *testing.T) {
	if !model.ValidArtifactName("foo-bar") {
		t.Fatal("foo-bar should be valid")
	}
	if model.ValidArtifactName("Foo") {
		t.Fatal("Foo should be invalid")
	}
	if model.ValidArtifactName("-foo") || model.ValidArtifactName("foo-") {
		t.Fatal("leading/trailing dash should be invalid")
	}
}

type resolutionStoreStub struct {
	store.Store
	artifact *model.Artifact
	versions map[string]*model.ArtifactVersion
	graph    []store.GraphData
	nextID   int64
	resolves []*model.Resolution
	items    [][]model.ResolutionItem
}

func (s *resolutionStoreStub) GetArtifactByName(name string) (*model.Artifact, error) {
	if s.artifact != nil && s.artifact.Name == name {
		return s.artifact, nil
	}
	return nil, stubNotFound
}

func (s *resolutionStoreStub) GetVersion(_ int64, version string) (*model.ArtifactVersion, error) {
	if v, ok := s.versions[version]; ok {
		return v, nil
	}
	return nil, stubNotFound
}

func (s *resolutionStoreStub) LoadGraph() ([]store.GraphData, error) { return s.graph, nil }

func (s *resolutionStoreStub) NextID() (int64, error) {
	s.nextID++
	return s.nextID, nil
}

func (s *resolutionStoreStub) SaveResolution(r *model.Resolution, items []model.ResolutionItem, _ time.Time) error {
	r.ID = int64(len(s.resolves) + 1)
	s.resolves = append(s.resolves, r)
	s.items = append(s.items, append([]model.ResolutionItem(nil), items...))
	return nil
}

func (s *resolutionStoreStub) AddChange(_ *model.ChangeRecord, _ time.Time) error { return nil }

var stubNotFound = &stubError{}

type stubError struct{}

func (*stubError) Error() string { return "not found" }

func TestResolveUnspecifiedVersionIsIndependentAndPersisted(t *testing.T) {
	artifact := &model.Artifact{ID: 1, Name: "cli"}
	v1 := &model.ArtifactVersion{ID: 11, Version: "1.0.0"}
	v2 := &model.ArtifactVersion{ID: 12, Version: "2.0.0"}
	stub := &resolutionStoreStub{
		artifact: artifact,
		versions: map[string]*model.ArtifactVersion{"1.0.0": v1, "2.0.0": v2},
		graph: []store.GraphData{{
			Artifact: *artifact,
			Versions: []store.VersionData{{Version: *v1}, {Version: *v2}},
		}},
	}
	svc := New(stub)

	firstVersion := "1.0.0"
	first, err := svc.Resolve("cli", &firstVersion)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if first.Status != model.ResolutionSucceeded || len(first.Resolved) != 1 || first.Resolved[0].Version != "1.0.0" {
		t.Fatalf("first response = %+v", first)
	}

	second, err := svc.Resolve("cli", nil)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if second.Status != model.ResolutionSucceeded || len(second.Resolved) != 1 || second.Resolved[0].Version != "2.0.0" {
		t.Fatalf("second response = %+v", second)
	}
	if len(stub.resolves) != 2 || len(stub.items) != 2 {
		t.Fatalf("persisted resolutions = %d, item batches = %d", len(stub.resolves), len(stub.items))
	}
	if !strings.Contains(stub.resolves[0].ResultJSON, `"1.0.0"`) || !strings.Contains(stub.resolves[1].ResultJSON, `"2.0.0"`) {
		t.Fatalf("resolution records = %+v", stub.resolves)
	}
	for i, want := range []string{"1.0.0", "2.0.0"} {
		if len(stub.items[i]) != 1 || stub.items[i][0].SelectedVersion != want {
			t.Fatalf("resolution %d items = %+v, want %s", i+1, stub.items[i], want)
		}
	}
}
