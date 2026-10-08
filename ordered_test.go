package tuiffects

import (
	"strconv"
	"testing"
	"time"
)

func orderedMapOf(n int) *orderedMap[int] {
	m := newOrderedMap[int]()
	for i := 0; i < n; i++ {
		v := i
		m.Set("ring-"+strconv.Itoa(i), &v)
	}
	return &m
}

// TestOrderedMapKeepsItsOrderPastTheIndex covers the map index an orderedMap
// adds once it is large. Order, replacement in place, auto ids and Clear must
// behave the same on both sides of the switch.
func TestOrderedMapKeepsItsOrderPastTheIndex(t *testing.T) {
	for _, n := range []int{3, orderedMapIndexAt, orderedMapIndexAt + 1, 100} {
		m := orderedMapOf(n)
		for i, key := range m.Keys() {
			if want := "ring-" + strconv.Itoa(i); key != want {
				t.Fatalf("n=%d: key %d is %q, want %q", n, i, key, want)
			}
			if got := m.Get(key); got == nil || *got != i {
				t.Fatalf("n=%d: Get(%q) = %v, want %d", n, key, got, i)
			}
		}
		if m.Has("missing") || m.Get("missing") != nil {
			t.Fatalf("n=%d: a missing key is found", n)
		}
		replaced := -1
		m.Set("ring-1", &replaced)
		if m.Keys()[1] != "ring-1" || *m.Get("ring-1") != -1 || m.Len() != n {
			t.Fatalf("n=%d: replacing a key moved it or grew the map", n)
		}
		zero := 0
		m.Set(strconv.Itoa(n+1), &zero)
		if id := m.nextAutoID(); id != strconv.Itoa(n+2) {
			t.Fatalf("n=%d: nextAutoID = %q, want %q", n, id, strconv.Itoa(n+2))
		}
		m.Clear()
		if m.Len() != 0 || m.Has("ring-0") {
			t.Fatalf("n=%d: Clear left keys behind", n)
		}
		m.Set("again", &zero)
		if m.Get("again") != &zero || m.Has("ring-0") {
			t.Fatalf("n=%d: the map after Clear is wrong", n)
		}
	}
}

// TestOrderedMapLookupDoesNotScan is the perf budget for a large orderedMap.
// rings gives each character one path per ring cell, so a character's path
// map holds hundreds of ids, and a linear lookup was 58% of rings' frame CPU.
//
// The test compares a lookup in a large map with one in a small map, so it
// holds on a slow or loaded machine. A scan makes the large lookup about 500
// times slower. An index keeps the two within a small factor.
//
// Negative control: with no index, the large lookup is 893 times the small
// one.
func TestOrderedMapLookupDoesNotScan(t *testing.T) {
	small, large := orderedMapOf(8), orderedMapOf(4096)
	smallKey, largeKey := "ring-7", "ring-4095"
	measure := func(m *orderedMap[int], key string) time.Duration {
		best := time.Duration(1<<63 - 1)
		for trial := 0; trial < 7; trial++ {
			start := time.Now()
			for i := 0; i < 20000; i++ {
				if m.Get(key) == nil {
					t.Fatal("key not found")
				}
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	smallTime, largeTime := measure(small, smallKey), measure(large, largeKey)
	ratio := float64(largeTime) / float64(max(smallTime, 1))
	t.Logf("20000 lookups: %v in 8 entries, %v in 4096 entries, ratio %.1f",
		smallTime, largeTime, ratio)
	if ratio > 20 {
		t.Errorf("a lookup in 4096 entries costs %.0f times one in 8, want under 20", ratio)
	}
}
