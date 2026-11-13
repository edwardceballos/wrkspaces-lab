package cmdpicker

import (
	"testing"

	"github.com/edwardceballos/control/internal/config"
)

func TestCommandItemsFilterByTag(t *testing.T) {
	cmds := []config.RunCmd{
		{Name: "mock", Tags: []string{"czc", "mock"}},
		{Name: "real", Tags: []string{"czc", "real", "apigee"}},
		{Name: "gcp", Tags: []string{"deploy", "gcp", "external-db"}},
	}

	items := commandItems(cmds, "czc")
	if len(items) != 2 {
		t.Fatalf("expected 2 czc items, got %d", len(items))
	}

	items = commandItems(cmds, "external-db")
	if len(items) != 1 {
		t.Fatalf("expected 1 external-db item, got %d", len(items))
	}
}

func TestCommandTagsAreSortedAndUnique(t *testing.T) {
	cmds := []config.RunCmd{
		{Name: "a", Tags: []string{"czc", "mock"}},
		{Name: "b", Tags: []string{"czc", "apigee"}},
	}

	got := commandTags(cmds)
	want := []string{"apigee", "czc", "mock"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
