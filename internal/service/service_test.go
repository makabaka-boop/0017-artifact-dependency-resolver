package service

import (
	"testing"

	"artifact-dep-resolver/internal/model"
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
