package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// magpie keeps Codex's own compaction point (model_auto_compact_token_limit)
// at the size set in magpie: the key Codex takes over the model list's
// window, which another tool had left at 900K on a 1M model — the session
// grew past the vendor's window before Codex's own count, magpie's too,
// reached the limit, and the compaction it then sent no longer fit and was
// refused. A size set in magpie is re-asserted; magpie's bare default
// yields to a value of the user's own, as Claude Code's window yields to
// autoCompactWindow (#876); stepping out of Codex takes magpie's own away.
func TestCodexCompactPointOwned(t *testing.T) {
	home, read := codexHome(t, `{"OPENAI_API_KEY":"sk-x"}`, "model = \"gpt-5.4\"\nmodel_auto_compact_token_limit = 900000\n")
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: "http://127.0.0.1:1/v1",
		Models: []string{"m1"}, Contexts: map[string]int{"m1": 1000000}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".codex", "config.toml")
	key := func() string {
		v, _ := edit.GetTOMLTop(path, "model_auto_compact_token_limit")
		return strings.TrimSpace(v)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// no size set in magpie: the user's own 900000 stands (magpie's default
	// doesn't take it over)
	cx := codex(home)
	must(cx.Fields[0].Set("fake/m1"))
	if key() != "900000" {
		t.Fatalf("the user's own: %q\n%s", key(), read())
	}
	// a size set in magpie (a provider's Compact at, here): written over it
	must(provider.SetModelCompacts("fake", map[string]int{"*": 500000}))
	must(cx.Sync())
	if key() != "500000" {
		t.Fatalf("magpie's 500K: %q\n%s", key(), read())
	}
	// something else changes it again: Check says so, Sync puts it back
	must(edit.SetTOMLTop(path, edit.KV{Path: "model_auto_compact_token_limit", Value: 900000}))
	if c := codex(home).Check(); !strings.Contains(c, "900000") || !strings.Contains(c, "500000") {
		t.Fatalf("check: %q", c)
	}
	must(codex(home).Sync())
	if key() != "500000" {
		t.Fatalf("after sync: %q\n%s", key(), read())
	}
	// magpie's size taken back: magpie's own comes away
	must(provider.SetModelCompacts("fake", nil))
	must(codex(home).Sync())
	if key() != "" {
		t.Fatalf("magpie's size taken back: %q\n%s", key(), read())
	}
	// a value of the user's own set again: stands, nothing typed in magpie
	must(edit.SetTOMLTop(path, edit.KV{Path: "model_auto_compact_token_limit", Value: 900000}))
	must(codex(home).Sync())
	if key() != "900000" {
		t.Fatalf("the user's own again: %q\n%s", key(), read())
	}
	// full window too (magpie's default suspended, not magpie's size)
	must(settings.Save(settings.Settings{FullContext: true}))
	must(codex(home).Sync())
	if key() != "900000" {
		t.Fatalf("full window took the user's own: %q", key())
	}
	// magpie steps out of Codex: a size of magpie's goes, the user's stays
	must(settings.Save(settings.Settings{}))
	must(provider.SetModelCompacts("fake", map[string]int{"*": 500000}))
	must(codex(home).Sync())
	if key() != "500000" {
		t.Fatalf("before stepping out: %q\n%s", key(), read())
	}
	must(codex(home).Fields[0].Set(""))
	if key() != "" {
		t.Fatalf("stepping out left magpie's own: %q", key())
	}
}

// The size magpie writes is the one it hands the model list too: the
// provider's or model's Compact at, else the one typed for every model —
// and it follows the model Codex is set to.
func TestCodexCompactPointFollowsModel(t *testing.T) {
	home, _ := codexHome(t, `{"OPENAI_API_KEY":"sk-x"}`, "model = \"gpt-5.4\"\n")
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: "http://127.0.0.1:1/v1",
		Models: []string{"m1", "m2"}, Contexts: map[string]int{"m1": 1000000, "m2": 200000}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".codex", "config.toml")
	key := func() string {
		v, _ := edit.GetTOMLTop(path, "model_auto_compact_token_limit")
		return strings.TrimSpace(v)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(provider.SetCompactAt(300000))
	cx := codex(home)
	must(cx.Fields[0].Set("fake/m1"))
	if key() != "300000" {
		t.Fatalf("typed in magpie: %q", key())
	}
	must(provider.SetModelCompacts("fake", map[string]int{"m1": 600000}))
	must(cx.Sync())
	if key() != "600000" {
		t.Fatalf("the model's own: %q", key())
	}
	// a model at or under the size: nothing to compact at (its window
	// stands), and magpie's own comes away
	must(cx.Fields[0].Set("fake/m2"))
	if key() != "" {
		t.Fatalf("a model under the size: %q", key())
	}
	must(cx.Fields[0].Set("fake/m1"))
	if key() != "600000" {
		t.Fatalf("back on m1: %q", key())
	}
}
