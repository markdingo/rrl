package rrl

import (
	"testing"
	"time"
)

func TestNowFunc(t *testing.T) {
	r := NewRRL(NewConfig())
	if r == nil {
		t.Fatal("NewRRL failed unexpectedly")
	}

	system := time.Now()
	ours := r.cfg.nowFunc()
	diff := ours.Sub(system).Abs() // Should be small
	if diff > time.Second {
		t.Error("Default time.Now() func differs from system time.Now()", system, ours, diff)
	}

	var myNow time.Time
	nowFunc := func() time.Time {
		myNow = myNow.Add(time.Second)
		return myNow
	}

	c := NewConfig()
	c.SetNowFunc(nowFunc)
	r = NewRRL(c)
	ours = r.cfg.nowFunc()
	diff = ours.Sub(system).Abs() // Should be large
	if diff < time.Hour*24*365*40 {
		t.Error("Our timeFunc does not differ from system time.Now()", system, ours, diff)
	}

	ourNext := r.cfg.nowFunc()
	diff = ourNext.Sub(ours)
	if diff != time.Second {
		t.Error("Our timeFunc is not ticking by one second per call", diff)
	}
}
