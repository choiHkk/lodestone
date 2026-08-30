package embed

import (
	"strings"
	"testing"

	"lodestone/internal/analyze"
)

func TestMakeUnitsKeepsRawSource(t *testing.T) {
	t.Parallel()

	const source = "func load() error { return nil }"
	units := makeUnits([]analyze.Function{{Source: source}}, Settings{})
	if len(units) != 1 || units[0].text != source {
		t.Fatalf("units = %+v", units)
	}
}

func TestMakeUnitsPrefixesEveryUnitAlike(t *testing.T) {
	t.Parallel()

	const prefix = "Instruct: retrieve equivalents\nQuery:"
	sources := []analyze.Function{{Source: "func read() {}"}, {Source: "func write() {}"}}
	units := makeUnits(sources, Settings{Instruction: prefix})

	if len(units) != len(sources) {
		t.Fatalf("units = %+v", units)
	}
	for index, unit := range units {
		want := prefix + sources[index].Source
		if unit.text != want {
			t.Errorf("unit %d text = %q, want %q", index, unit.text, want)
		}
	}
}

func TestNamespaceIncludesModelProfile(t *testing.T) {
	t.Parallel()

	namespace := Namespace(Settings{Model: "/missing/model", Profile: "granite", MaxTokens: 768})
	if !strings.HasPrefix(namespace, "granite|") {
		t.Fatalf("namespace = %q", namespace)
	}
}
