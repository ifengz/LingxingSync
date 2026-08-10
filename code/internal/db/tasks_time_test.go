package db

import (
	"testing"
	"time"
)

func TestNormalizeDBTimeUsesSessionUTCOffset(t *testing.T) {
	raw := time.Date(2026, time.August, 10, 12, 30, 0, 0, time.UTC)

	if got, want := normalizeDBTime(raw, 8*time.Hour), time.Date(2026, time.August, 10, 4, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("normalize +08:00 time = %s, want %s", got, want)
	}
	if got := normalizeDBTime(raw, 0); !got.Equal(raw) {
		t.Fatalf("normalize UTC time = %s, want unchanged %s", got, raw)
	}
}

func TestNormalizeTaskTimesPreservesDuration(t *testing.T) {
	started := time.Date(2026, time.August, 10, 12, 30, 0, 0, time.UTC)
	finished := started.Add(17 * time.Second)
	task := Task{StartedAt: &started, FinishedAt: &finished, CreatedAt: started}

	normalizeTaskTimes(&task, 8*time.Hour)

	if got := task.FinishedAt.Sub(*task.StartedAt); got != 17*time.Second {
		t.Fatalf("normalized task duration = %s, want 17s", got)
	}
	wantStarted := time.Date(2026, time.August, 10, 4, 30, 0, 0, time.UTC)
	if !task.StartedAt.Equal(wantStarted) || !task.CreatedAt.Equal(wantStarted) {
		t.Fatalf("normalized task times = started %s, created %s; want %s", task.StartedAt, task.CreatedAt, wantStarted)
	}
}
