package store

import (
	"testing"
	"time"

	"artifact-dep-resolver/internal/model"
)

func TestSaveLockfileAndEntries(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	a, _ := st.CreateArtifact("foo", now)
	v, _ := st.CreateVersion(a.ID, "1.0.0", now)
	r := &model.Resolution{
		RequestRef:     "resolve-1",
		RootArtifactID: a.ID,
		InputJSON:      `{"artifact":"foo"}`,
		Status:         "succeeded",
		ResultJSON:     `[]`,
		Source:         "direct",
	}
	if err := st.SaveResolution(r, nil, now); err != nil {
		t.Fatal(err)
	}
	lf := &model.Lockfile{
		Name:               "lk",
		RootArtifactID:     a.ID,
		SourceResolutionID: r.ID,
	}
	if err := st.SaveLockfile(lf, []model.LockfileEntry{
		{ArtifactID: a.ID, VersionID: v.ID, SelectedVersion: "1.0.0"},
	}, now); err != nil {
		t.Fatal(err)
	}
	if lf.ID == 0 {
		t.Fatal("lockfile id not set")
	}
	got, err := st.GetLockfileByName("lk")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != lf.ID {
		t.Fatalf("id mismatch")
	}
	entries, err := st.ListLockfileEntries(lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].SelectedVersion != "1.0.0" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestLockfileUniqueName(t *testing.T) {
	st := newMemStore(t)
	now := time.Now()
	a, _ := st.CreateArtifact("foo", now)
	r := &model.Resolution{RequestRef: "r", RootArtifactID: a.ID, InputJSON: "{}", Status: "succeeded", ResultJSON: "[]", Source: "direct"}
	if err := st.SaveResolution(r, nil, now); err != nil {
		t.Fatal(err)
	}
	lf := &model.Lockfile{Name: "dup", RootArtifactID: a.ID, SourceResolutionID: r.ID}
	if err := st.SaveLockfile(lf, nil, now); err != nil {
		t.Fatal(err)
	}
	lf2 := &model.Lockfile{Name: "dup", RootArtifactID: a.ID, SourceResolutionID: r.ID}
	if err := st.SaveLockfile(lf2, nil, now); err == nil {
		t.Fatal("expected duplicate name error")
	}
}
