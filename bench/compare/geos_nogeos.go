//go:build !geos

package compare

import (
	"errors"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// errGEOSUnavailable is returned by every geosImpl method in a build
// without the "geos" tag. Callers must check Available() and skip (not
// fail) rather than relying on this error, but returning a descriptive
// error here (instead of panicking) keeps the stub safe to call by
// accident.
var errGEOSUnavailable = errors.New("compare: geos implementation not built; rebuild with -tags geos and a system libgeos")

// geosImpl is the no-cgo stub used when built without -tags geos. It
// satisfies Impl so bench/compare and its tests compile and run (skipping
// geos sub-benchmarks/checks) on any machine, including CI runners and
// contributors without libgeos-dev installed.
type geosImpl struct{}

// NewGEOS returns the geos stub adapter (Available() == false). Build with
// -tags geos to get the real go-geos-backed implementation instead.
func NewGEOS() Impl { return geosImpl{} }

func (geosImpl) Name() string    { return "geos" }
func (geosImpl) Available() bool { return false }

func (geosImpl) Convert(g geom.Geometry) (Handle, error) { return nil, errGEOSUnavailable }

func (geosImpl) Intersection(a, b Handle) (Handle, error) { return nil, errGEOSUnavailable }
func (geosImpl) Union(a, b Handle) (Handle, error)        { return nil, errGEOSUnavailable }
func (geosImpl) Difference(a, b Handle) (Handle, error)   { return nil, errGEOSUnavailable }

func (geosImpl) Relate(a, b Handle) (string, error) { return "", errGEOSUnavailable }

func (geosImpl) Area(g Handle) (float64, error)   { return 0, errGEOSUnavailable }
func (geosImpl) Length(g Handle) (float64, error) { return 0, errGEOSUnavailable }

func (geosImpl) Buffer(g Handle, dist float64) (Handle, error) { return nil, errGEOSUnavailable }

func (geosImpl) Prepare(g Handle) (Handle, error) { return nil, errGEOSUnavailable }
func (geosImpl) PreparedIntersects(prep, g Handle) (bool, error) {
	return false, errGEOSUnavailable
}
