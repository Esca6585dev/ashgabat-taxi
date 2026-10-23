package services

import (
	"testing"
	"time"

	"github.com/ashgabat-taxi/backend/internal/models"
)

func TestDispatchStartForScheduledRides(t *testing.T) {
	now := time.Now()
	created := now.Add(-2 * time.Hour)

	plain := &models.Order{CreatedAt: created}
	if !DispatchStart(plain).Equal(created) || !DispatchDue(plain, now) {
		t.Fatal("normal ride must dispatch from creation")
	}

	later := now.Add(time.Hour)
	booked := &models.Order{CreatedAt: created, ScheduledAt: &later}
	if DispatchDue(booked, now) {
		t.Fatal("ride booked in 1h offered too early")
	}

	// booked 2h ago for 10 min from now: due, and the timeout counts from 5 min ago, not 2h ago
	soon := now.Add(10 * time.Minute)
	booked.ScheduledAt = &soon
	start := DispatchStart(booked)
	if !DispatchDue(booked, now) || now.Sub(start) > 6*time.Minute {
		t.Fatalf("booked ride: due=%v, searching for %v", DispatchDue(booked, now), now.Sub(start))
	}
}
