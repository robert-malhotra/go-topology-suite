package geom

import (
	"sync"
	"testing"

	"github.com/exergy-dev/go-topology-suite/crs"
	"github.com/stretchr/testify/assert"
)

// TestEnvelopeCacheConcurrent exercises the atomic.Pointer envelope cache
// under -race. Multiple goroutines compute Envelope() simultaneously; the
// CAS dance must produce a single consistent value with no data race.
func TestEnvelopeCacheConcurrent(t *testing.T) {
	pts := make([]XY, 1000)
	for i := range pts {
		pts[i] = XY{float64(i), float64(i % 7)}
	}
	ls := NewLineString(crs.WGS84, pts)

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	envs := make([]Envelope, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			envs[i] = ls.Envelope()
		}(i)
	}
	wg.Wait()

	want := envs[0]
	for i, e := range envs {
		assert.Equal(t, want, e, "envelope inconsistency at %d", i)
	}
}
