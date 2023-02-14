package cache

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCacheEvictFail(t *testing.T) {
	c := New(4)
	var evictCount int
	c.SetEvict(func(i interface{}) bool {
		evictCount++
		return false
	})
	addf := func(interface{}) interface{} {
		return nil
	}
	updatef := func() interface{} {
		return nil
	}

	start := time.Now()
	// Fill up at least one of the shards
	var err error
	for ix := 0; ix < 1000000; ix++ { // Should stop at less than 256 * 4 iterations
		ret := c.UpdateAdd(fmt.Sprintf("before-%d", ix), addf, updatef)
		e, ok := ret.(error)
		if ok {
			err = e
			break
		}
	}
	if evictCount == 0 || (err == nil) || !strings.Contains(err.Error(), "shard full") {
		t.Fatal("Setup not working as intended", evictCount, err)
	}
	end := time.Now()
	elapse := end.Sub(start)

	// Start up a go-routine to monitor this go-routine for a lock stall.
	ch := make(chan int)
	defer close(ch)
	go waitOrFatal(t, ch, time.Second+elapse*10)

	// Now keep adding until we hit the same shard again (with its lock still set)
	// It's kindof a guess that this iteration count catches the same shard as it
	// relies on go's map hash function as to how keys are distributed across
	// shards. But empirically this is way way more than enough with the minimal
	// number of shards.
	for ix := 0; ix < 1000000; ix++ {
		c.UpdateAdd(fmt.Sprintf("after-%d", ix), addf, updatef)
	}
}

func waitOrFatal(t *testing.T, ch chan int, delay time.Duration) {
	select {
	case <-ch:
		return
	case <-time.After(delay):
		panic("Escape from stalled Lock")
	}
}
