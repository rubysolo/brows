package main

import (
	"strings"
	"testing"

	"github.com/masterminds/semver"
)

func TestAsString(t *testing.T) {
	v := "hello"
	if asString(&v) != "hello" {
		t.Fatalf("expected 'hello', got %q", asString(&v))
	}

	if asString(nil) != "" {
		t.Fatalf("expected empty string for nil pointer")
	}
}

func TestSortedTags(t *testing.T) {
	tags := []string{"1.2.0", "1.1.0", "2.0.0"}
	list := sortedTags(tags)
	if len(list) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(list))
	}

	if list[0].Original() != "1.1.0" {
		t.Fatalf("expected first tag 1.1.0, got %s", list[0].Original())
	}
	if list[2].Original() != "2.0.0" {
		t.Fatalf("expected last tag 2.0.0, got %s", list[2].Original())
	}
}

func TestFindTagIndex(t *testing.T) {
	list := sortedTags([]string{"1.2.0", "1.3.0", "2.0.0"})
	cur, _ := semver.NewVersion("1.1.0")
	idx, err := findTagIndex(cur, list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 0 {
		t.Fatalf("expected index 0, got %d", idx)
	}

	cur2, _ := semver.NewVersion("3.0.0")
	_, err = findTagIndex(cur2, list)
	if err == nil {
		t.Fatalf("expected error when no greater tag found")
	}
}

func TestIsMajorMinorPatch(t *testing.T) {
	maj, _ := semver.NewVersion("2.0.0")
	min, _ := semver.NewVersion("2.1.0")
	pat, _ := semver.NewVersion("2.1.1")
	pre, _ := semver.NewVersion("2.0.0-beta.1")

	if !isMajor(maj) {
		t.Fatalf("expected major for 2.0.0")
	}
	if !isMinor(min) {
		t.Fatalf("expected minor for 2.1.0")
	}
	if !isPatch(pat) {
		t.Fatalf("expected patch for 2.1.1")
	}
	if isMajor(pre) {
		t.Fatalf("expected prerelease not to be major")
	}
}

func TestClampMinMax(t *testing.T) {
	if min(2, 3) != 2 {
		t.Fatalf("min failed")
	}
	if max(2, 3) != 3 {
		t.Fatalf("max failed")
	}
	if clamp(5, 1, 3) != 3 {
		t.Fatalf("clamp failed (5 -> 3)")
	}
	if clamp(-1, 0, 10) != 0 {
		t.Fatalf("clamp failed (-1 -> 0)")
	}
}

func TestInitialModel(t *testing.T) {
	m := initialModel(nil, "owner", "repo", "1.2.3")
	if m.owner != "owner" || m.repo != "repo" {
		t.Fatalf("owner/repo mismatch: %s/%s", m.owner, m.repo)
	}
	if m.version.Original() != "1.2.3" {
		t.Fatalf("version mismatch: %s", m.version.Original())
	}
	if m.loaded {
		t.Fatalf("expected loaded=false by default")
	}
	if m.focus != -1 {
		t.Fatalf("expected focus -1 by default, got %d", m.focus)
	}
}

func TestReleaseListVisualization(t *testing.T) {
	m := initialModel(nil, "owner", "repo", "1.0.0")
	m.tagList = sortedTags([]string{"1.0.0", "1.1.0", "1.1.1", "2.0.0"})
	m.focus = 1
	m.viewport.Width = 80

	s := m.releaseList()
	if !strings.Contains(s, "▇") {
		t.Fatalf("expected major glyph ▇ in release list, got %q", s)
	}
	if !strings.Contains(s, "▅") {
		t.Fatalf("expected minor glyph ▅ in release list, got %q", s)
	}
	if !strings.Contains(s, "▂") {
		t.Fatalf("expected patch glyph ▂ in release list, got %q", s)
	}
}

func TestHeaderViewContainsRepo(t *testing.T) {
	m := initialModel(nil, "alice", "hello", "0.1.0")
	m.viewport.Width = 80
	h := m.headerView()
	if !strings.Contains(h, "alice/hello") {
		t.Fatalf("expected header to contain owner/repo, got %q", h)
	}
}

func TestClampSwap(t *testing.T) {
	// swap if high < low
	if clamp(5, 10, 1) != 5 {
		t.Fatalf("clamp failed swap case")
	}
}

func TestBodyAndFooterView(t *testing.T) {
	m := initialModel(nil, "owner", "repo", "0.1.0")
	m.viewport.Width = 40
	m.viewport.Height = 10

	// unloaded state shows loading
	m.loaded = false
	b := m.bodyView()
	if !strings.Contains(b, "loading...") {
		t.Fatalf("expected loading message in body, got %q", b)
	}

	// loaded shows viewport content
	m.loaded = true
	m.viewport.SetContent("hello from viewport")
	b2 := m.bodyView()
	if !strings.Contains(b2, "hello from viewport") {
		t.Fatalf("expected viewport content in body, got %q", b2)
	}

	// Footer when no scrolling (visible >= total) returns a simple line
	m.viewport.SetContent("short")
	f := m.footerView()
	if !strings.Contains(f, "─") {
		t.Fatalf("expected footer line in footer view, got %q", f)
	}

	// Footer when scrollable should include a percent
	long := strings.Repeat("line\n", 200)
	m.viewport.SetContent(long)
	f2 := m.footerView()
	if !strings.Contains(f2, "%") {
		t.Fatalf("expected percent indicator in footer view, got %q", f2)
	}
}
