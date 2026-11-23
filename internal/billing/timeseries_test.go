package billing

import (
	"errors"
	"testing"
	"time"
)

func TestBuildTimeseriesBucketsAlignsAndClampsBoundaries(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load Asia/Shanghai: %v", err)
	}

	t.Run("hourly 24h window", func(t *testing.T) {
		start := time.Date(2026, 11, 22, 11, 0, 0, 0, shanghai)
		end := start.Add(24 * time.Hour)
		buckets, err := buildTimeseriesBuckets(start, end, GranularityHour, shanghai)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(buckets) != 24 {
			t.Fatalf("len(buckets) = %d, want 24", len(buckets))
		}
		if !buckets[0].BucketStart.Equal(start) || !buckets[23].PeriodEnd.Equal(end) {
			t.Fatalf("unexpected bounds: first=%s lastEnd=%s", buckets[0].BucketStart, buckets[23].PeriodEnd)
		}
	})

	t.Run("monthly window clamps partial first and last month", func(t *testing.T) {
		start := time.Date(2026, 10, 15, 0, 0, 0, 0, shanghai)
		end := time.Date(2026, 11, 24, 0, 0, 0, 0, shanghai)
		buckets, err := buildTimeseriesBuckets(start, end, GranularityMonth, shanghai)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(buckets) != 2 {
			t.Fatalf("len(buckets) = %d, want 2", len(buckets))
		}
		wantOctBucket := time.Date(2026, 10, 1, 0, 0, 0, 0, shanghai)
		wantNovStart := time.Date(2026, 11, 1, 0, 0, 0, 0, shanghai)
		if !buckets[0].BucketStart.Equal(wantOctBucket) || !buckets[0].PeriodStart.Equal(start) || !buckets[0].PeriodEnd.Equal(wantNovStart) {
			t.Fatalf("october bucket = %+v", buckets[0])
		}
		if !buckets[1].BucketStart.Equal(wantNovStart) || !buckets[1].PeriodStart.Equal(wantNovStart) || !buckets[1].PeriodEnd.Equal(end) {
			t.Fatalf("november bucket = %+v", buckets[1])
		}
	})

	t.Run("DST spring-forward 23h and fall-back 25h days in America/New_York", func(t *testing.T) {
		ny, err := time.LoadLocation("America/New_York")
		if err != nil {
			t.Fatalf("load America/New_York: %v", err)
		}
		springStart := localDayStart(2026, 3, 8, ny)
		springEnd := localDayStart(2026, 3, 9, ny)
		springHours, err := buildTimeseriesBuckets(springStart, springEnd, GranularityHour, ny)
		if err != nil || len(springHours) != 23 {
			t.Fatalf("spring-forward hours = %d, err = %v, want 23", len(springHours), err)
		}

		fallStart := localDayStart(2026, 11, 1, ny)
		fallEnd := localDayStart(2026, 11, 2, ny)
		fallHours, err := buildTimeseriesBuckets(fallStart, fallEnd, GranularityHour, ny)
		if err != nil || len(fallHours) != 25 {
			t.Fatalf("fall-back hours = %d, err = %v, want 25", len(fallHours), err)
		}
		fallDays, err := buildTimeseriesBuckets(fallStart, fallEnd, GranularityDay, ny)
		if err != nil || len(fallDays) != 1 {
			t.Fatalf("fall-back days = %d, err = %v, want 1", len(fallDays), err)
		}
	})

	t.Run("midnight spring-forward in America/Santiago does not loop and produces exact daily buckets", func(t *testing.T) {
		santiago, err := time.LoadLocation("America/Santiago")
		if err != nil {
			t.Fatalf("load America/Santiago: %v", err)
		}
		start := localDayStart(2026, 9, 5, santiago)
		end := localDayStart(2026, 9, 8, santiago)
		days, err := buildTimeseriesBuckets(start, end, GranularityDay, santiago)
		if err != nil {
			t.Fatalf("santiago daily buckets error: %v", err)
		}
		if len(days) != 3 {
			t.Fatalf("len(days) = %d, want 3", len(days))
		}
		// 2026-09-06 00:00 jumps to 01:00 -03:00 (04:00Z)
		if days[1].BucketStart.In(santiago).Day() != 6 || days[1].BucketStart.In(santiago).Hour() != 1 {
			t.Fatalf("santiago DST day start = %s, want 2026-09-06 01:00", days[1].BucketStart.In(santiago))
		}
	})

	t.Run("half-hour zones Asia/Kolkata and Australia/Lord_Howe align to local hour boundaries", func(t *testing.T) {
		kolkata, err := time.LoadLocation("Asia/Kolkata")
		if err != nil {
			t.Fatalf("load Asia/Kolkata: %v", err)
		}
		kStart := localDayStart(2026, 11, 23, kolkata)
		kEnd := localDayStart(2026, 11, 24, kolkata)
		kHours, err := buildTimeseriesBuckets(kStart, kEnd, GranularityHour, kolkata)
		if err != nil || len(kHours) != 24 {
			t.Fatalf("kolkata hours = %d, err = %v, want 24", len(kHours), err)
		}
		for i, b := range kHours {
			if b.BucketStart.In(kolkata).Minute() != 0 {
				t.Fatalf("kolkata bucket %d minute = %d (%s), want 0", i, b.BucketStart.In(kolkata).Minute(), b.BucketStart.In(kolkata))
			}
		}

		lordHowe, err := time.LoadLocation("Australia/Lord_Howe")
		if err != nil {
			t.Fatalf("load Australia/Lord_Howe: %v", err)
		}
		lhStart := localDayStart(2026, 10, 4, lordHowe)
		lhEnd := localDayStart(2026, 10, 5, lordHowe)
		lhHours, err := buildTimeseriesBuckets(lhStart, lhEnd, GranularityHour, lordHowe)
		if err != nil {
			t.Fatalf("lord_howe hours error: %v", err)
		}
		// After the 02:00->02:30 jump, subsequent hours (03:00..23:00) must return to :00 rather than drifting to :30.
		lastBucketLocal := lhHours[len(lhHours)-1].BucketStart.In(lordHowe)
		if lastBucketLocal.Hour() != 23 || lastBucketLocal.Minute() != 0 {
			t.Fatalf("lord_howe last bucket = %s, want 23:00", lastBucketLocal)
		}
	})

	t.Run("rejects unbounded hourly range", func(t *testing.T) {
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, shanghai)
		end := time.Date(2027, 1, 1, 0, 0, 0, 0, shanghai)
		if _, err := buildTimeseriesBuckets(start, end, GranularityHour, shanghai); !errors.Is(err, ErrPeriodTooWideForGranularity) {
			t.Fatalf("err = %v, want ErrPeriodTooWideForGranularity", err)
		}
	})
}
