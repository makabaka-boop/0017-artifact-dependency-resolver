package store

import (
	"path/filepath"
	"testing"
	"time"

	"artifact-dep-resolver/internal/model"
)

func newMemStore(t *testing.T) *sqliteStore {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestArtifactUniqueConstraint(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	if _, err := st.CreateArtifact("foo", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateArtifact("foo", now); err == nil {
		t.Fatal("expected unique constraint violation")
	}
}

func TestVersionTransactionAndChange(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	a, _ := st.CreateArtifact("foo", now)
	if _, err := st.CreateVersion(a.ID, "1.0.0", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateVersion(a.ID, "1.0.0", now); err == nil {
		t.Fatal("expected duplicate version error")
	}
}

func TestPagination(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		if _, err := st.CreateArtifact(n, now); err != nil {
			t.Fatal(err)
		}
	}
	arts, err := st.ListArtifacts(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Fatalf("len = %d", len(arts))
	}
}

func TestResultJSONReplay(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	a, _ := st.CreateArtifact("foo", now)
	v, _ := st.CreateVersion(a.ID, "1.0.0", now)
	r := &model.Resolution{
		RequestRef:     "resolve-1",
		RootArtifactID: a.ID,
		InputJSON:      `{"artifact":"foo"}`,
		Status:         "succeeded",
		ResultJSON:     `[{"ArtifactName":"foo","Version":"1.0.0"}]`,
		Source:         "direct",
	}
	if err := st.SaveResolution(r, []model.ResolutionItem{
		{ArtifactID: a.ID, VersionID: v.ID, SelectedVersion: "1.0.0", Depth: 0, Reason: "selected"},
	}, now); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetResolution(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultJSON != r.ResultJSON {
		t.Fatalf("result_json mismatch: %q vs %q", got.ResultJSON, r.ResultJSON)
	}
}

func TestChangeRecordAppendOnly(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	c := &model.ChangeRecord{EntityType: "artifact", EntityID: 1, Action: "create", BeforeJSON: "", AfterJSON: "{}"}
	if err := st.AddChange(c, now); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListChanges(10, 0, "artifact", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
}
