package benchfix

import (
	"testing"

	"github.com/exergy-dev/go-topology-suite/validate"
)

func TestNGonValid(t *testing.T) {
	for _, n := range []int{3, 8, 64, 1024, 8192} {
		p := NGon(n, 10)
		if err := validate.Validate(p); err != nil {
			t.Errorf("NGon(%d, 10) invalid: %v", n, err)
		}
	}
}

func TestStarValid(t *testing.T) {
	for _, n := range []int{8, 16, 64, 1024, 4096, 8192} {
		p := Star(n, 100, 60)
		if err := validate.Validate(p); err != nil {
			t.Errorf("Star(%d, 100, 60) invalid: %v", n, err)
		}
	}
}

func TestGridCount(t *testing.T) {
	for _, tc := range []struct{ rows, cols int }{{4, 4}, {8, 8}, {16, 16}} {
		mp := Grid(tc.rows, tc.cols, 0.1)
		want := tc.rows * tc.cols
		if got := mp.NumGeometries(); got != want {
			t.Errorf("Grid(%d,%d,0.1): got %d polygons, want %d", tc.rows, tc.cols, got, want)
		}
	}
}

func TestRotatePreservesValidity(t *testing.T) {
	star := Star(64, 100, 60)
	rotated := Rotate(star, 0.35)
	if rotated.NumRings() != star.NumRings() {
		t.Fatalf("Rotate changed ring count: got %d, want %d", rotated.NumRings(), star.NumRings())
	}
	if err := validate.Validate(rotated); err != nil {
		t.Errorf("Rotate(Star(64,100,60), 0.35) invalid: %v", err)
	}

	ngon := NGon(32, 10)
	rotatedNGon := Rotate(ngon, 1.1)
	if err := validate.Validate(rotatedNGon); err != nil {
		t.Errorf("Rotate(NGon(32,10), 1.1) invalid: %v", err)
	}
}
