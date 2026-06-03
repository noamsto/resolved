package cache

import (
	"testing"
	"time"

	"github.com/noamsto/resolved/internal/model"
)

func TestPutGetFresh(t *testing.T) {
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	c := New(t.TempDir())
	c.now = func() time.Time { return now }

	c.Put("o/r#1", model.Status{State: "open", Title: "x", UpdatedAt: now})
	got, ok := c.Get("o/r#1")
	if !ok || got.State != "open" {
		t.Fatalf("Get fresh = %+v ok=%v", got, ok)
	}
}

func TestOpenExpiresAfterOneHour(t *testing.T) {
	base := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	c := New(t.TempDir())
	c.now = func() time.Time { return base }
	c.Put("o/r#1", model.Status{State: "open", UpdatedAt: base})

	c.now = func() time.Time { return base.Add(2 * time.Hour) }
	if _, ok := c.Get("o/r#1"); ok {
		t.Fatal("open entry should expire after 1h")
	}
}

func TestClosedSurvivesDays(t *testing.T) {
	base := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	c := New(t.TempDir())
	c.now = func() time.Time { return base }
	c.Put("o/r#1", model.Status{State: "closed", UpdatedAt: base})

	c.now = func() time.Time { return base.Add(3 * 24 * time.Hour) }
	if _, ok := c.Get("o/r#1"); !ok {
		t.Fatal("closed entry should survive 3 days")
	}
}

func TestPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

	c1 := New(dir)
	c1.now = func() time.Time { return base }
	c1.Put("o/r#1", model.Status{State: "closed", UpdatedAt: base})

	c2 := New(dir)
	c2.now = func() time.Time { return base }
	if _, ok := c2.Get("o/r#1"); !ok {
		t.Fatal("entry should persist to disk and reload")
	}
}

func TestDisabledAlwaysMisses(t *testing.T) {
	c := Disabled()
	c.Put("o/r#1", model.Status{State: "closed"})
	if _, ok := c.Get("o/r#1"); ok {
		t.Fatal("disabled cache should always miss")
	}
}
