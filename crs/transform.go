package crs

import (
	"errors"
	"math"
)

// ErrUntransformable is returned by OperationFor when one of the operands
// lacks a Definition (or has a Definition without enough information to
// compute the requested transform).
var ErrUntransformable = errors.New("crs: CRS has no Definition; cannot transform")

// Operation is a coordinate-mapping function from one CRS's storage
// representation to another's. Forward operates on raw geometry-storage
// coordinates: degrees for geographic CRSes, projection units (typically
// metres) for projected CRSes, axis-order honoured.
//
// Used by gts.Transform (in the top-level package, which can import
// both crs and geom). The crs package itself stays free of any geom
// dependency to avoid an import cycle.
type Operation interface {
	Forward(x, y float64) (float64, float64)
}

// OperationFor returns the Operation that maps coordinates from src to
// dst. Both CRSes must carry a Definition unless src and dst are equal,
// in which case Forward is the identity.
func OperationFor(src, dst *CRS) (Operation, error) {
	if Equal(src, dst) {
		return identityOp{}, nil
	}
	if src == nil || dst == nil {
		return nil, ErrUntransformable
	}
	if src.definition == nil || dst.definition == nil {
		return nil, ErrUntransformable
	}
	return &pipelineOp{
		src:             src.definition,
		dst:             dst.definition,
		needsDatumShift: src.definition.Datum.Name != dst.definition.Datum.Name,
	}, nil
}

// identityOp is the Operation for src==dst: pass coordinates through.
type identityOp struct{}

func (identityOp) Forward(x, y float64) (float64, float64) { return x, y }

// pipelineOp is the general source→target operation. It walks every
// stage of the standard projection pipeline, skipping no-op stages.
// needsDatumShift is precomputed at construction so the per-coordinate
// path doesn't repeat the datum-name string compare.
type pipelineOp struct {
	src, dst        *Definition
	needsDatumShift bool
}

const (
	deg2rad = math.Pi / 180.0
	rad2deg = 180.0 / math.Pi
)

func (p *pipelineOp) Forward(x, y float64) (float64, float64) {
	// Stage 1: source storage → source geographic (lon, lat) in radians.
	var lonRad, latRad float64
	if p.src.Projection != nil {
		lonRad, latRad = p.src.Projection.Inverse(x, y)
	} else {
		lonDeg, latDeg := x, y
		if p.src.AxisOrder == AxisLatLon {
			lonDeg, latDeg = y, x
		}
		lonRad = lonDeg * deg2rad
		latRad = latDeg * deg2rad
	}

	// Stage 2: datum shift via geocentric.
	if p.needsDatumShift {
		lonRad, latRad, _ = shiftDatum(lonRad, latRad, 0, p.src.Datum, p.dst.Datum)
	}

	// Stage 3: target geographic → target storage.
	if p.dst.Projection != nil {
		return p.dst.Projection.Forward(lonRad, latRad)
	}
	lonDeg, latDeg := lonRad*rad2deg, latRad*rad2deg
	if p.dst.AxisOrder == AxisLatLon {
		return latDeg, lonDeg
	}
	return lonDeg, latDeg
}
