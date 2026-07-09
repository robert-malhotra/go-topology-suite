package geodesic

import "math"

// Karney's geodesic inverse problem solver, ported from
// geographiclib-c (geodesic.c::geod_geninverse_int and helpers,
// MIT/X11 licensed). Unlike Vincenty's iteration this converges for
// every pair of surface points, including near-antipodal cases where
// Vincenty fails.
//
// We use this only as a fallback for vincentyInverse() — the existing
// Vincenty code is faster on the common short-edge case and we keep
// it as the primary path. When Vincenty fails to converge (typically
// pairs within ~0.5° of antipodal) we fall through to karneyInverse.

const (
	karneyTol0   = 2.220446049250313e-16 // DBL_EPSILON
	karneyTol1   = 200 * karneyTol0
	karneyTol2   = 1.4901161193847656e-08 // sqrt(DBL_EPSILON)
	karneyTolb   = karneyTol0             // tolerance for bisection
	karneyMaxit1 = 20
	karneyMaxit2 = 83 // = maxit1 + DBL_MANT_DIG + 10 = 20 + 53 + 10
)

// a1m1f: Karney's A1-1 series in eps² (geographiclib-c::A1m1f).
func a1m1f(eps float64) float64 {
	coeff := []float64{1, 4, 64, 0, 256}
	m := karneyOrder / 2
	t := polyval(m, coeff, eps*eps) / coeff[m+1]
	return (t + eps) / (1 - eps)
}

// a2m1f: A2-1 series (geographiclib-c::A2m1f).
func a2m1f(eps float64) float64 {
	coeff := []float64{-11, -28, -192, 0, 256}
	m := karneyOrder / 2
	t := polyval(m, coeff, eps*eps) / coeff[m+1]
	return (t - eps) / (1 + eps)
}

// c1f fills c[1..karneyOrder] with C1 Fourier coefficients (sin series).
func c1f(eps float64, c *[karneyOrder + 1]float64) {
	coeff := []float64{
		-1, 6, -16, 32,
		-9, 64, -128, 2048,
		9, -16, 768,
		3, -5, 512,
		-7, 1280,
		-7, 2048,
	}
	eps2 := eps * eps
	d := eps
	o := 0
	for l := 1; l <= karneyOrder; l++ {
		m := (karneyOrder - l) / 2
		c[l] = d * polyval(m, coeff[o:], eps2) / coeff[o+m+1]
		o += m + 2
		d *= eps
	}
}

// c2f fills c[1..karneyOrder] with C2 Fourier coefficients (sin series).
func c2f(eps float64, c *[karneyOrder + 1]float64) {
	coeff := []float64{
		1, 2, 16, 32,
		35, 64, 384, 2048,
		15, 80, 768,
		7, 35, 512,
		63, 1280,
		77, 2048,
	}
	eps2 := eps * eps
	d := eps
	o := 0
	for l := 1; l <= karneyOrder; l++ {
		m := (karneyOrder - l) / 2
		c[l] = d * polyval(m, coeff[o:], eps2) / coeff[o+m+1]
		o += m + 2
		d *= eps
	}
}

// sumSinSeriesC1: sum_{i=1..karneyOrder} c[i] * sin(2i·sigma) for the C1
// or C2 series. (Different from the C3 sin series which begins at i=1
// already; here c[0] is unused.)
func sumSinSeriesC1(sinx, cosx float64, c *[karneyOrder + 1]float64) float64 {
	// Translate geographiclib-c SinCosSeries(TRUE, sinx, cosx, c, nC1):
	// y = sum_{i=1..nC1} c[i] · sin(2i·x).
	// c is indexed [0..nC1]; c[0] unused.
	n := karneyOrder
	ar := 2 * (cosx - sinx) * (cosx + sinx)
	var y0, y1 float64
	idx := n + 1 // pointer one past last
	if (n & 1) != 0 {
		idx--
		y0 = c[idx]
	}
	// halve n
	half := n / 2
	for half > 0 {
		half--
		idx--
		y1 = ar*y0 - y1 + c[idx]
		idx--
		y0 = ar*y1 - y0 + c[idx]
	}
	return 2 * sinx * cosx * y0
}

// karneyLengths returns s12b = (distance/b) and (optionally) m12b/m0.
// Mirrors geographiclib-c::Lengths with the GEOD_DISTANCE flag. We
// only need s12b (distance) for the fallback distance/bearing; if the
// caller needs m12, we compute it too (used by InverseStart astroid
// branch with f<0; for f>0, geographiclib's Inverse for f<0 isn't
// exercised here).
func karneyLengths(eps, sig12 float64,
	ssig1, csig1, dn1, ssig2, csig2, dn2, cbet1, cbet2 float64,
	wantM12 bool,
) (s12b, m12b, m0 float64) {
	var Ca [karneyOrder + 1]float64
	var Cb [karneyOrder + 1]float64
	A1 := a1m1f(eps)
	c1f(eps, &Ca)
	A2 := a2m1f(eps)
	c2f(eps, &Cb)
	m0 = A1 - A2
	A2 = 1 + A2
	A1 = 1 + A1
	B1 := sumSinSeriesC1(ssig2, csig2, &Ca) - sumSinSeriesC1(ssig1, csig1, &Ca)
	s12b = A1 * (sig12 + B1)
	if wantM12 {
		B2 := sumSinSeriesC1(ssig2, csig2, &Cb) - sumSinSeriesC1(ssig1, csig1, &Cb)
		J12 := m0*sig12 + (A1*B1 - A2*B2)
		m12b = dn2*(csig1*ssig2) - dn1*(ssig1*csig2) - csig1*csig2*J12
	}
	_ = dn2
	_ = dn1
	return
}

// astroid solves k⁴+2k³-(x²+y²-1)k²-2y²k-y² = 0 for the positive root.
// Direct port of geographiclib-c::Astroid.
func astroid(x, y float64) float64 {
	p := x * x
	q := y * y
	r := (p + q - 1) / 6
	if !(q == 0 && r <= 0) {
		S := p * q / 4
		r2 := r * r
		r3 := r * r2
		disc := S * (S + 2*r3)
		u := r
		if disc >= 0 {
			T3 := S + r3
			if T3 < 0 {
				T3 -= math.Sqrt(disc)
			} else {
				T3 += math.Sqrt(disc)
			}
			T := math.Cbrt(T3)
			if T != 0 {
				u += T + r2/T
			} else {
				u += T
			}
		} else {
			ang := math.Atan2(math.Sqrt(-disc), -(S + r3))
			u += 2 * r * math.Cos(ang/3)
		}
		v := math.Sqrt(u*u + q)
		var uv float64
		if u < 0 {
			uv = q / (v - u)
		} else {
			uv = u + v
		}
		w := (uv - q) / (2 * v)
		return uv / (math.Sqrt(uv+w*w) + w)
	}
	return 0
}

// inverseStart returns (sig12, salp1, calp1[, salp2, calp2, dnm]).
// sig12 < 0 means caller should run Newton; sig12 ≥ 0 means short
// line and salp2,calp2 are filled in too.
func inverseStart(
	sbet1, cbet1, dn1, sbet2, cbet2, dn2,
	lam12, slam12, clam12 float64,
) (sig12, salp1, calp1, salp2, calp2, dnm float64) {
	sig12 = -1
	sbet12 := sbet2*cbet1 - cbet2*sbet1
	cbet12 := cbet2*cbet1 + sbet2*sbet1
	sbet12a := sbet2*cbet1 + cbet2*sbet1
	shortline := cbet12 >= 0 && sbet12 < 0.5 && cbet2*lam12 < 0.5

	var somg12, comg12 float64
	if shortline {
		sbetm2 := (sbet1 + sbet2) * (sbet1 + sbet2)
		sbetm2 /= sbetm2 + (cbet1+cbet2)*(cbet1+cbet2)
		dnm = math.Sqrt(1 + karney.ep2*sbetm2)
		omg12 := lam12 / ((1 - karney.f) * dnm)
		somg12, comg12 = math.Sincos(omg12)
	} else {
		somg12, comg12 = slam12, clam12
	}

	salp1 = cbet2 * somg12
	if comg12 >= 0 {
		calp1 = sbet12 + cbet2*sbet1*somg12*somg12/(1+comg12)
	} else {
		calp1 = sbet12a - cbet2*sbet1*somg12*somg12/(1-comg12)
	}
	ssig12 := math.Hypot(salp1, calp1)
	csig12 := sbet1*sbet2 + cbet1*cbet2*comg12

	// etol2 from geographiclib-c::geod_init.
	tol2 := karneyTol2
	etol2 := 0.1 * tol2 / math.Sqrt(math.Max(0.001, math.Abs(karney.f))*math.Min(1.0, 1-karney.f/2)/2)

	if shortline && ssig12 < etol2 {
		// really short
		salp2 = cbet1 * somg12
		var t float64
		if comg12 >= 0 {
			t = somg12 * somg12 / (1 + comg12)
		} else {
			t = 1 - comg12
		}
		calp2 = sbet12 - cbet1*sbet2*t
		norm2(&salp2, &calp2)
		sig12 = math.Atan2(ssig12, csig12)
		return
	}

	// astroid branch: only for f >= 0 and near-antipodal.
	if math.Abs(karney.n) > 0.1 || csig12 >= 0 ||
		ssig12 >= 6*math.Abs(karney.n)*math.Pi*cbet1*cbet1 {
		// Zeroth-order spherical guess is OK; salp1, calp1 already set.
	} else {
		// Near-antipodal, f >= 0 path (WGS84).
		var x, y, lamscale, betscale float64
		lam12x := math.Atan2(-slam12, -clam12) // = lam12 - π
		k2 := sbet1 * sbet1 * karney.ep2
		eps := k2 / (2*(1+math.Sqrt(1+k2)) + k2)
		lamscale = karney.f * cbet1 * karney.a3f(eps) * math.Pi
		betscale = lamscale * cbet1
		x = lam12x / lamscale
		y = sbet12a / betscale
		xthresh := 1000 * tol2
		if y > -karneyTol1 && x > -1-xthresh {
			salp1 = math.Min(1.0, -x)
			calp1 = -math.Sqrt(1 - salp1*salp1)
		} else {
			k := astroid(x, y)
			omg12a := lamscale * (-x * k / (1 + k))
			somg12, comg12 := math.Sin(omg12a), -math.Cos(omg12a)
			salp1 = cbet2 * somg12
			calp1 = sbet12a - cbet2*sbet1*somg12*somg12/(1-comg12)
		}
	}
	if !(salp1 <= 0) {
		norm2(&salp1, &calp1)
	} else {
		salp1, calp1 = 1, 0
	}
	_ = dn1
	_ = dn2
	return
}

// lambda12 evaluates the residual v = lambda(alp1) - lam12 along with
// (when diffp is true) its derivative dv/dalp1, and updates a few
// auxiliary-sphere quantities. Direct port of geographiclib-c::Lambda12.
func lambda12(
	sbet1, cbet1, dn1, sbet2, cbet2, dn2 float64,
	salp1, calp1 float64,
	slam120, clam120 float64,
	diffp bool,
) (lam12, salp2, calp2, sig12, ssig1, csig1, ssig2, csig2, eps, domg12, dlam12 float64) {
	if sbet1 == 0 && calp1 == 0 {
		// break degeneracy
		calp1 = -1e-300
	}
	salp0 := salp1 * cbet1
	calp0 := math.Hypot(calp1, salp1*sbet1)

	ssig1 = sbet1
	somg1 := salp0 * sbet1
	csig1 = calp1 * cbet1
	comg1 := csig1
	norm2(&ssig1, &csig1)

	if cbet2 != cbet1 {
		salp2 = salp0 / cbet2
	} else {
		salp2 = salp1
	}
	if cbet2 != cbet1 || math.Abs(sbet2) != -sbet1 {
		var t float64
		if cbet1 < -sbet1 {
			t = (cbet2 - cbet1) * (cbet1 + cbet2)
		} else {
			t = (sbet1 - sbet2) * (sbet1 + sbet2)
		}
		calp2 = math.Sqrt(calp1*cbet1*calp1*cbet1+t) / cbet2
	} else {
		calp2 = math.Abs(calp1)
	}
	ssig2 = sbet2
	somg2 := salp0 * sbet2
	csig2 = calp2 * cbet2
	comg2 := csig2
	norm2(&ssig2, &csig2)

	sig12 = math.Atan2(math.Max(0.0, csig1*ssig2-ssig1*csig2),
		csig1*csig2+ssig1*ssig2)
	somg12 := math.Max(0.0, comg1*somg2-somg1*comg2)
	comg12 := comg1*comg2 + somg1*somg2
	eta := math.Atan2(somg12*clam120-comg12*slam120,
		comg12*clam120+somg12*slam120)
	k2 := calp0 * calp0 * karney.ep2
	eps = k2 / (2*(1+math.Sqrt(1+k2)) + k2)
	var Ca [karneyOrder]float64
	karney.c3f(eps, &Ca)
	B312 := sinCosSeriesSin(ssig2, csig2, Ca[:]) -
		sinCosSeriesSin(ssig1, csig1, Ca[:])
	domg12 = -karney.f * karney.a3f(eps) * salp0 * (sig12 + B312)
	lam12 = eta + domg12

	if diffp {
		if calp2 == 0 {
			dlam12 = -2 * (1 - karney.f) * dn1 / sbet1
		} else {
			_, m12b, _ := karneyLengths(eps, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2, cbet1, cbet2, true)
			dlam12 = m12b
			dlam12 *= (1 - karney.f) / (calp2 * cbet2)
		}
	}
	return
}

// karneyInverse solves the inverse problem on WGS84, returning
// distance s in metres, initial bearing α₁ (radians), and final
// bearing α₂ (radians). Always converges (uses input normalisation
// + Newton with bracketing fallback). Inputs are degrees.
func karneyInverse(lon1, lat1, lon2, lat2 float64) (s, alpha1, alpha2 float64) {
	a := SemiMajorA
	f := karney.f
	b := a * (1 - f)
	ep2 := karney.ep2

	// Input normalisation.
	dlon := lon2 - lon1
	for dlon > 180 {
		dlon -= 360
	}
	for dlon <= -180 {
		dlon += 360
	}
	lonsign := 1.0
	if dlon < 0 {
		lonsign = -1
		dlon = -dlon
	}
	swapp := 1.0
	if math.Abs(lat1) < math.Abs(lat2) {
		swapp = -1
		lonsign = -lonsign
		lat1, lat2 = lat2, lat1
	}
	latsign := -1.0
	if lat1 < 0 {
		latsign = 1
	}
	lat1 *= latsign
	lat2 *= latsign

	la1 := lat1 * math.Pi / 180
	la2 := lat2 * math.Pi / 180
	lam12 := dlon * math.Pi / 180
	slam12, clam12 := math.Sincos(lam12)

	tiny := math.SmallestNonzeroFloat64
	sbet1 := (1 - f) * math.Sin(la1)
	cbet1 := math.Cos(la1)
	norm2(&sbet1, &cbet1)
	if cbet1 < tiny {
		cbet1 = tiny
	}
	sbet2 := (1 - f) * math.Sin(la2)
	cbet2 := math.Cos(la2)
	norm2(&sbet2, &cbet2)
	if cbet2 < tiny {
		cbet2 = tiny
	}
	dn1 := math.Sqrt(1 + ep2*sbet1*sbet1)
	dn2 := math.Sqrt(1 + ep2*sbet2*sbet2)

	var s12x, sig12 float64
	var salp1, calp1, salp2, calp2 float64
	meridian := lat1 == -90 || slam12 == 0

	if meridian {
		// Geodesic might lie on a meridian.
		calp1, salp1 = clam12, slam12
		calp2, salp2 = 1, 0
		ssig1 := sbet1
		csig1 := calp1 * cbet1
		ssig2 := sbet2
		csig2 := calp2 * cbet2
		sig12 = math.Atan2(math.Max(0.0, csig1*ssig2-ssig1*csig2),
			csig1*csig2+ssig1*ssig2)
		s12x_, m12b, _ := karneyLengths(karney.n, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2, cbet1, cbet2, true)
		s12x = s12x_
		if sig12 < karneyTol2 || m12b >= 0 {
			if sig12 < 3*tiny {
				// The C original also zeroes sig12/m12x here; this
				// port only carries s12x forward.
				s12x = 0
			}
			s12x *= b
		} else {
			meridian = false
		}
	}

	if !meridian && sbet1 == 0 && (f <= 0 || (math.Pi-lam12) >= f*math.Pi) {
		// Geodesic along equator.
		calp1, calp2 = 0, 0
		salp1, salp2 = 1, 1
		s12x = a * lam12
	} else if !meridian {
		var dnm float64
		sig12, salp1, calp1, salp2, calp2, dnm = inverseStart(
			sbet1, cbet1, dn1, sbet2, cbet2, dn2,
			lam12, slam12, clam12)
		if sig12 >= 0 {
			s12x = sig12 * b * dnm
		} else {
			// Newton's method.
			numit := 0
			salp1a, calp1a := tiny, 1.0
			salp1b, calp1b := tiny, -1.0
			tripn, tripb := false, false
			var ssig1, csig1, ssig2, csig2, eps float64
			for {
				v, salp2v, calp2v, sig12v, ssig1v, csig1v, ssig2v, csig2v, epsv, _, dv :=
					lambda12(sbet1, cbet1, dn1, sbet2, cbet2, dn2,
						salp1, calp1, slam12, clam12, numit < karneyMaxit1)
				salp2, calp2 = salp2v, calp2v
				sig12 = sig12v
				ssig1, csig1, ssig2, csig2 = ssig1v, csig1v, ssig2v, csig2v
				eps = epsv
				tol := 8.0
				if !tripn {
					tol = 1.0
				}
				if tripb || !(math.Abs(v) >= tol*karneyTol0) || numit == karneyMaxit2 {
					break
				}
				if v > 0 && (numit > karneyMaxit1 || calp1/salp1 > calp1b/salp1b) {
					salp1b, calp1b = salp1, calp1
				} else if v < 0 && (numit > karneyMaxit1 || calp1/salp1 < calp1a/salp1a) {
					salp1a, calp1a = salp1, calp1
				}
				numit++
				if numit < karneyMaxit1 && dv > 0 {
					dalp1 := -v / dv
					if math.Abs(dalp1) < math.Pi {
						sd, cd := math.Sincos(dalp1)
						nsalp1 := salp1*cd + calp1*sd
						if nsalp1 > 0 {
							calp1 = calp1*cd - salp1*sd
							salp1 = nsalp1
							norm2(&salp1, &calp1)
							tripn = math.Abs(v) <= 16*karneyTol0
							continue
						}
					}
				}
				salp1 = (salp1a + salp1b) / 2
				calp1 = (calp1a + calp1b) / 2
				norm2(&salp1, &calp1)
				tripn = false
				tripb = math.Abs(salp1a-salp1)+(calp1a-calp1) < karneyTolb ||
					math.Abs(salp1-salp1b)+(calp1-calp1b) < karneyTolb
			}
			s12x_, _, _ := karneyLengths(eps, sig12, ssig1, csig1, dn1, ssig2, csig2, dn2, cbet1, cbet2, false)
			s12x = s12x_ * b
		}
	}
	s = s12x

	// Restore signs absorbed by normalization.
	if swapp < 0 {
		salp1, salp2 = salp2, salp1
		calp1, calp2 = calp2, calp1
	}
	salp1 *= swapp * lonsign
	calp1 *= swapp * latsign
	salp2 *= swapp * lonsign
	calp2 *= swapp * latsign

	alpha1 = math.Atan2(salp1, calp1)
	alpha2 = math.Atan2(salp2, calp2)
	return
}
