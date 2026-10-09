package nscale

import (
	"testing"
	"time"
)

func TestErrorGraceTolerate(t *testing.T) {
	grace := ErrorGrace{Period: 50 * time.Millisecond}

	if grace.Tolerate(false) {
		t.Fatalf("Tolerate(false) = true, want false for a non-failing poll")
	}
	if !grace.Tolerate(true) {
		t.Fatalf("Tolerate(true) = false on the first failing poll, want true")
	}

	time.Sleep(60 * time.Millisecond)
	if grace.Tolerate(true) {
		t.Fatalf("Tolerate(true) = true after the period elapsed, want false")
	}

	grace.Tolerate(false)
	if !grace.Tolerate(true) {
		t.Fatalf("Tolerate(true) = false after a non-failing poll, want the run to restart")
	}
}
