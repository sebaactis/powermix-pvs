package timeutil

import (
	"testing"
	"time"
)

func TestFormat_Zero(t *testing.T) {
	if Format(time.Time{}) != "" {
		t.Fatal("zero time debe ser string vacio")
	}
}

func TestFormat_UTCToArgentina(t *testing.T) {
	// 15:04:05 UTC → 12:04:05 ART (UTC-3, sin DST)
	in := time.Date(2026, 7, 10, 15, 4, 5, 0, time.UTC)
	if got := Format(in); got != "2026-07-10 12:04:05" {
		t.Fatalf("Format = %q", got)
	}
}

func TestNow_InArgentina(t *testing.T) {
	n := Now()
	name, off := n.Zone()
	if off != -3*3600 {
		t.Fatalf("offset = %d (%s), want UTC-3", off, name)
	}
}

func TestWithDSNTimezone(t *testing.T) {
	if got := WithDSNTimezone(""); got != "" {
		t.Fatalf("empty dsn = %q", got)
	}
	base := "postgres://u:p@h:5432/db"
	want := base + "?timezone=America/Argentina/Buenos_Aires"
	if got := WithDSNTimezone(base); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	withQ := base + "?sslmode=require"
	want2 := withQ + "&timezone=America/Argentina/Buenos_Aires"
	if got := WithDSNTimezone(withQ); got != want2 {
		t.Fatalf("got %q want %q", got, want2)
	}
	already := base + "?timezone=UTC"
	if got := WithDSNTimezone(already); got != already {
		t.Fatalf("should keep existing timezone, got %q", got)
	}
}
