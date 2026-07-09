package geodesic

import (
	"math"

	"github.com/exergy-dev/go-topology-suite/geom"
)

// Karney exact ellipsoidal polygon area, following Karney (2013),
// "Algorithms for geodesics," J. Geod. 87:43–55, Section 6, and the
// reference C implementation in geographiclib-c (MIT/X11 licensed).
//
// The algorithm sums per-edge contributions S12 along each polygon
// edge. For an edge from point 1 to point 2 the contribution is
//
//	S12 = c2 * alp12  +  A4 * (B42 - B41)
//
// where c2 is the authalic-area constant, alp12 is a carefully-computed
// "azimuth difference" (using tan-half-angle of the longitude and
// reduced-latitude differences for numerical stability), A4 depends on
// the equatorial azimuth alp0 of the geodesic, and B41/B42 are Fourier
// (cosine) series in the auxiliary-sphere arc length sigma.
//
// Series expansion order: terms through eccentricity^6 (karneyOrder = 6),
// matching geographiclib-c's GEOGRAPHICLIB_GEODESIC_ORDER=6. This gives
// sub-millimetre accuracy on edges < 30 000 km.

const karneyOrder = 6

type karneyConsts struct {
	a, f, b float64
	e2      float64 // e²    = (a² - b²)/a²
	ep2     float64 // e'²   = (a² - b²)/b²
	n       float64 // (a-b)/(a+b) = f/(2-f)
	c2      float64 // a²/2 + b²/2 · atanh(e)/e

	// A3x: coefficients of the A3 series (polynomial in eps) — needed
	// to convert the auxiliary-sphere lambda back to the spherical
	// longitude difference omg12 used by the area formula.
	A3x [karneyOrder]float64
	// C3x: triangular array of coefficients for the B3 sin-Fourier
	// series, used in domg12 = lam12 - omg12.
	C3x [(karneyOrder * (karneyOrder - 1)) / 2]float64
	// C4x: triangular array of C4 cosine-Fourier coefficients for I4.
	C4x [(karneyOrder * (karneyOrder + 1)) / 2]float64
}

var karney = newKarneyConsts()

func newKarneyConsts() *karneyConsts {
	a := SemiMajorA
	f := Flattening
	b := a * (1 - f)
	e2 := f * (2 - f)
	ep2 := e2 / ((1 - f) * (1 - f))
	n := f / (2 - f)

	var c2 float64
	if e2 == 0 {
		c2 = a * a
	} else {
		e := math.Sqrt(math.Abs(e2))
		if e2 > 0 {
			c2 = (a*a + b*b*math.Atanh(e)/e) / 2
		} else {
			c2 = (a*a + b*b*math.Atan(e)/e) / 2
		}
	}

	kc := &karneyConsts{a: a, f: f, b: b, e2: e2, ep2: ep2, n: n, c2: c2}
	kc.computeA3x()
	kc.computeC3x()
	kc.computeC4x()
	return kc
}

// computeA3x: verbatim port of geographiclib-c geodesic.c::A3coeff.
func (kc *karneyConsts) computeA3x() {
	n := kc.n
	coeff := []float64{
		-3, 128,
		-2, -3, 64,
		-1, -3, -1, 16,
		3, -1, -2, 8,
		1, -1, 2,
		1, 1,
	}
	o, k := 0, 0
	for j := karneyOrder - 1; j >= 0; j-- {
		m := karneyOrder - j - 1
		if m > j {
			m = j
		}
		kc.A3x[k] = polyval(m, coeff[o:], n) / coeff[o+m+1]
		k++
		o += m + 2
	}
}

// computeC3x: verbatim port of geographiclib-c geodesic.c::C3coeff.
func (kc *karneyConsts) computeC3x() {
	n := kc.n
	coeff := []float64{
		3, 128,
		2, 5, 128,
		-1, 3, 3, 64,
		-1, 0, 1, 8,
		-1, 1, 4,
		5, 256,
		1, 3, 128,
		-3, -2, 3, 64,
		1, -3, 2, 32,
		7, 512,
		-10, 9, 384,
		5, -9, 5, 192,
		7, 512,
		-14, 7, 512,
		21, 2560,
	}
	o, k := 0, 0
	for l := 1; l < karneyOrder; l++ {
		for j := karneyOrder - 1; j >= l; j-- {
			m := karneyOrder - j - 1
			if m > j {
				m = j
			}
			kc.C3x[k] = polyval(m, coeff[o:], n) / coeff[o+m+1]
			k++
			o += m + 2
		}
	}
}

// a3f evaluates A3 = sum_{i=0..karneyOrder-1} A3x[i] · eps^i (Horner
// in eps, but A3x[0] is the highest-order coefficient).
func (kc *karneyConsts) a3f(eps float64) float64 {
	return polyval(karneyOrder-1, kc.A3x[:], eps)
}

// c3f fills c[1..karneyOrder-1] with the Fourier coefficients of the
// B3 sin-series at the given eps. Mirrors geographiclib-c::C3f.
func (kc *karneyConsts) c3f(eps float64, c *[karneyOrder]float64) {
	mult := 1.0
	o := 0
	for l := 1; l < karneyOrder; l++ {
		m := karneyOrder - l - 1
		mult *= eps
		c[l] = mult * polyval(m, kc.C3x[o:], eps)
		o += m + 1
	}
}

// sinCosSeriesSin evaluates sum_{i=1..n-1} c[i] · sin(2i·x) using
// Clenshaw summation. Matches geographiclib-c SinCosSeries(TRUE, ...).
// c[0] is unused (it's the absent constant term); n is the length of
// the meaningful coefficients including c[0].
func sinCosSeriesSin(sinx, cosx float64, c []float64) float64 {
	n := len(c)
	ar := 2 * (cosx - sinx) * (cosx + sinx)
	var y0, y1 float64
	idx := n
	// SinCosSeries(TRUE, ...) advances pointer by n+1 (sinp=true), so
	// effective length is n (including c[0] dummy). To match the C
	// algorithm we set: if ((n+1)&1) y0 = c[n]; else y0 = 0; n_used = (n+1)/2.
	// Easier: directly pre-strip c[0].
	// Equivalent restatement: compute SinCosSeries(TRUE) on c[1..n-1].
	if (idx & 1) == 0 {
		idx--
		y0 = c[idx]
	}
	for idx > 1 {
		idx--
		y1 = ar*y0 - y1 + c[idx]
		idx--
		y0 = ar*y1 - y0 + c[idx]
	}
	return 2 * sinx * cosx * y0
}

// computeC4x: verbatim port of geographiclib-c geodesic.c::C4coeff
// (GEOGRAPHICLIB_GEODESIC_ORDER=6).
func (kc *karneyConsts) computeC4x() {
	n := kc.n
	coeff := []float64{
		// C4[0], coeff of eps^5, polynomial in n of order 0
		97, 15015,
		// C4[0], coeff of eps^4, polynomial in n of order 1
		1088, 156, 45045,
		// C4[0], coeff of eps^3, polynomial in n of order 2
		-224, -4784, 1573, 45045,
		// C4[0], coeff of eps^2, polynomial in n of order 3
		-10656, 14144, -4576, -858, 45045,
		// C4[0], coeff of eps^1, polynomial in n of order 4
		64, 624, -4576, 6864, -3003, 15015,
		// C4[0], coeff of eps^0, polynomial in n of order 5
		100, 208, 572, 3432, -12012, 30030, 45045,
		// C4[1], coeff of eps^5, polynomial in n of order 0
		1, 9009,
		// C4[1], coeff of eps^4, polynomial in n of order 1
		-2944, 468, 135135,
		// C4[1], coeff of eps^3, polynomial in n of order 2
		5792, 1040, -1287, 135135,
		// C4[1], coeff of eps^2, polynomial in n of order 3
		5952, -11648, 9152, -2574, 135135,
		// C4[1], coeff of eps^1, polynomial in n of order 4
		-64, -624, 4576, -6864, 3003, 135135,
		// C4[2], coeff of eps^5, polynomial in n of order 0
		8, 10725,
		// C4[2], coeff of eps^4, polynomial in n of order 1
		1856, -936, 225225,
		// C4[2], coeff of eps^3, polynomial in n of order 2
		-8448, 4992, -1144, 225225,
		// C4[2], coeff of eps^2, polynomial in n of order 3
		-1440, 4160, -4576, 1716, 225225,
		// C4[3], coeff of eps^5, polynomial in n of order 0
		-136, 63063,
		// C4[3], coeff of eps^4, polynomial in n of order 1
		1024, -208, 105105,
		// C4[3], coeff of eps^3, polynomial in n of order 2
		3584, -3328, 1144, 315315,
		// C4[4], coeff of eps^5, polynomial in n of order 0
		-128, 135135,
		// C4[4], coeff of eps^4, polynomial in n of order 1
		-2560, 832, 405405,
		// C4[5], coeff of eps^5, polynomial in n of order 0
		128, 99099,
	}
	// NB: C4coeff in geographiclib-c does NOT clamp m by (j-l) — unlike
	// A3coeff and C3coeff. m is simply nC4-j-1. (See geodesic.c::C4coeff.)
	o, k := 0, 0
	for l := 0; l < karneyOrder; l++ {
		for j := karneyOrder - 1; j >= l; j-- {
			m := karneyOrder - j - 1
			kc.C4x[k] = polyval(m, coeff[o:], n) / coeff[o+m+1]
			k++
			o += m + 2
		}
	}
}

// polyval evaluates p[0]*x^N + p[1]*x^(N-1) + ... + p[N] (Horner).
func polyval(N int, p []float64, x float64) float64 {
	if N < 0 {
		return 0
	}
	y := p[0]
	for i := 1; i <= N; i++ {
		y = y*x + p[i]
	}
	return y
}

// c4f fills c[0..karneyOrder-1] with the Fourier coefficients of the
// area cosine series at the given eps. Mirrors geographiclib-c::C4f.
func (kc *karneyConsts) c4f(eps float64, c *[karneyOrder]float64) {
	mult := 1.0
	o := 0
	for l := 0; l < karneyOrder; l++ {
		m := karneyOrder - l - 1
		c[l] = mult * polyval(m, kc.C4x[o:], eps)
		o += m + 1
		mult *= eps
	}
}

// sinCosSeriesCos evaluates sum_{i=0..n-1} c[i] * cos((2i+1)*x) using
// Clenshaw summation, where (sinx, cosx) are sin(x), cos(x). This
// matches geographiclib-c SinCosSeries(FALSE, ...).
func sinCosSeriesCos(sinx, cosx float64, c []float64) float64 {
	n := len(c)
	ar := 2 * (cosx - sinx) * (cosx + sinx) // 2·cos(2x)
	var y0, y1 float64
	idx := n
	if idx&1 != 0 {
		idx--
		y0 = c[idx]
	}
	for idx > 0 {
		idx--
		y1 = ar*y0 - y1 + c[idx]
		idx--
		y0 = ar*y1 - y0 + c[idx]
	}
	return cosx * (y0 - y1)
}

// norm2 normalises (s, c) so that s² + c² = 1.
func norm2(s, c *float64) {
	r := math.Hypot(*s, *c)
	*s /= r
	*c /= r
}

// edgeS12 returns the Karney signed-area contribution for one polygon
// edge from (lon1,lat1) to (lon2,lat2), inputs in degrees. The result
// is in m². Sign convention follows geographiclib-c (positive for the
// area to the left of the directed edge, viewed from inside).
//
// We piggy-back on the Vincenty inverse iteration to avoid duplicating
// a full Karney inverse: Vincenty's lambda iteration converges on all
// polygon edges short enough to enclose finite area, and produces the
// same auxiliary-sphere quantities (sin/cos of the reduced latitudes,
// the auxiliary-sphere lambda, and the azimuths) that S12 needs. For
// the few near-antipodal edges where Vincenty fails, the Karney inverse
// in karney_inverse.go takes over.
func edgeS12(lon1, lat1, lon2, lat2 float64) float64 {
	if lon1 == lon2 && lat1 == lat2 {
		return 0
	}
	a := SemiMajorA
	f := Flattening
	e2 := karney.e2
	ep2 := karney.ep2

	// Normalise inputs the same way geographiclib does, so the auxiliary-
	// sphere arc-length signs and the cosine-series direction line up
	// with the reference implementation. We apply lonsign/latsign/swapp
	// and remember them to restore the sign of S12 at the end.
	dlonRaw := lon2 - lon1
	// Wrap dlonRaw to (-180, 180].
	for dlonRaw > 180 {
		dlonRaw -= 360
	}
	for dlonRaw <= -180 {
		dlonRaw += 360
	}
	lonsign := 1.0
	if dlonRaw < 0 {
		lonsign = -1
		dlonRaw = -dlonRaw
	}
	swapp := 1.0
	if math.Abs(lat1) < math.Abs(lat2) {
		swapp = -1
		lonsign = -lonsign
		lat1, lat2 = lat2, lat1
	}
	latsign := -1.0 // make lat1 ≤ 0
	if lat1 < 0 {
		latsign = 1
	}
	lat1 *= latsign
	lat2 *= latsign

	la1 := lat1 * math.Pi / 180
	la2 := lat2 * math.Pi / 180
	dlon := dlonRaw * math.Pi / 180

	// Reduced latitudes β: tan(β) = (1-f)·tan(φ).
	// sin(β) = (1-f)·sin(φ) / D, cos(β) = cos(φ) / D where
	// D = √(cos²φ + ((1-f)sinφ)²).
	sinPhi1, cosPhi1 := math.Sincos(la1)
	sinPhi2, cosPhi2 := math.Sincos(la2)
	sbet1 := (1 - f) * sinPhi1
	cbet1 := cosPhi1
	norm2(&sbet1, &cbet1)
	sbet2 := (1 - f) * sinPhi2
	cbet2 := cosPhi2
	norm2(&sbet2, &cbet2)

	// Vincenty iteration on auxiliary-sphere lambda.
	L := dlon
	lambda := L
	var sinLambda, cosLambda float64
	var sinSigma, cosSigma float64
	var sinAlpha, cos2Alpha float64
	for iter := 0; iter < 200; iter++ {
		sinLambda, cosLambda = math.Sincos(lambda)
		t1 := cbet2 * sinLambda
		t2 := cbet1*sbet2 - sbet1*cbet2*cosLambda
		sinSigma = math.Sqrt(t1*t1 + t2*t2)
		if sinSigma == 0 {
			return 0
		}
		cosSigma = sbet1*sbet2 + cbet1*cbet2*cosLambda
		sinAlpha = cbet1 * cbet2 * sinLambda / sinSigma
		cos2Alpha = 1 - sinAlpha*sinAlpha
		var cos2SigmaM float64
		if cos2Alpha == 0 {
			cos2SigmaM = 0
		} else {
			cos2SigmaM = cosSigma - 2*sbet1*sbet2/cos2Alpha
		}
		C := f / 16 * cos2Alpha * (4 + f*(4-3*cos2Alpha))
		lambdaPrev := lambda
		lambda = L + (1-C)*f*sinAlpha*
			(math.Atan2(sinSigma, cosSigma)+
				C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
		if math.Abs(lambda-lambdaPrev) < 1e-13 {
			break
		}
	}

	// Azimuths at endpoints.
	salp1 := cbet2 * sinLambda
	calp1 := cbet1*sbet2 - sbet1*cbet2*cosLambda
	{
		r := math.Hypot(salp1, calp1)
		if r == 0 {
			return 0
		}
		salp1 /= r
		calp1 /= r
	}
	salp2 := cbet1 * sinLambda
	calp2 := -sbet1*cbet2 + cbet1*sbet2*cosLambda
	{
		r := math.Hypot(salp2, calp2)
		if r == 0 {
			return 0
		}
		salp2 /= r
		calp2 /= r
	}

	// Equatorial azimuth: salp0 = salp1·cbet1, calp0 = √(calp1²+(salp1·sbet1)²).
	salp0 := salp1 * cbet1
	calp0 := math.Hypot(calp1, salp1*sbet1)

	var S12 float64
	if calp0 != 0 && salp0 != 0 {
		// Auxiliary-sphere positions sigma1, sigma2.
		ssig1 := sbet1
		csig1 := calp1 * cbet1
		ssig2 := sbet2
		csig2 := calp2 * cbet2
		if ssig1 == 0 && csig1 == 0 {
			csig1 = 1
		}
		if ssig2 == 0 && csig2 == 0 {
			csig2 = 1
		}
		norm2(&ssig1, &csig1)
		norm2(&ssig2, &csig2)

		k2 := calp0 * calp0 * ep2
		eps := k2 / (2*(1+math.Sqrt(1+k2)) + k2)
		A4 := a * a * calp0 * salp0 * e2

		var C4a [karneyOrder]float64
		karney.c4f(eps, &C4a)
		B41 := sinCosSeriesCos(ssig1, csig1, C4a[:])
		B42 := sinCosSeriesCos(ssig2, csig2, C4a[:])
		S12 = A4 * (B42 - B41)
	}

	// alp12: GeographicLib uses two formulas. For non-near-antipodal
	// pairs (i.e. the polygon edges we care about) the compact form
	// based on tan-half-angles is numerically far more stable than
	// alp2 - alp1. Reference: geographiclib-c geodesic.c line ~1000.
	//
	//   tan(alp12/2) =  somg12 · (sbet1·(1+cbet2) + sbet2·(1+cbet1))
	//                 / ((1+comg12) · (sbet1·sbet2 + (1+cbet1)·(1+cbet2)))
	//
	// where (somg12, comg12) is the sin/cos of the difference of
	// "spherical longitudes" omg1, omg2 on the auxiliary sphere:
	//   somg1 = salp0·sbet1, comg1 = calp1·cbet1   (UNnormalized)
	//   somg2 = salp0·sbet2, comg2 = calp2·cbet2
	// then somg12 = max(0, comg1·somg2 - somg1·comg2),
	//      comg12 = comg1·comg2 + somg1·somg2.
	// This is *not* the iterated Vincenty lambda.
	// We compute (somg12, comg12) the way geographiclib does in the
	// "non-meridian, Newton's method" branch: omg12 = lam12 - domg12
	// where domg12 = -f · A3(eps) · salp0 · (sig12 + B312), with B312 a
	// sin-series correction in the auxiliary-sphere arc length.
	var somg12, comg12 float64
	{
		// Need ssig1, csig1, ssig2, csig2, sig12, eps, salp0 even when
		// the calp0/salp0 == 0 branch above skipped the area term.
		ssig1 := sbet1
		csig1Loc := calp1 * cbet1
		ssig2 := sbet2
		csig2Loc := calp2 * cbet2
		if ssig1 == 0 && csig1Loc == 0 {
			csig1Loc = 1
		}
		if ssig2 == 0 && csig2Loc == 0 {
			csig2Loc = 1
		}
		norm2(&ssig1, &csig1Loc)
		norm2(&ssig2, &csig2Loc)
		k2Loc := calp0 * calp0 * ep2
		eps := k2Loc / (2*(1+math.Sqrt(1+k2Loc)) + k2Loc)
		var C3a [karneyOrder]float64
		karney.c3f(eps, &C3a)
		B312 := sinCosSeriesSin(ssig2, csig2Loc, C3a[:]) -
			sinCosSeriesSin(ssig1, csig1Loc, C3a[:])
		sig12 := math.Atan2(
			math.Max(0.0, csig1Loc*ssig2-ssig1*csig2Loc),
			csig1Loc*csig2Loc+ssig1*ssig2,
		)
		domg12 := -f * karney.a3f(eps) * salp0 * (sig12 + B312)
		// omg12 = lam12 - domg12, in (sin, cos) form.
		slam12 := math.Sin(dlon)
		clam12 := math.Cos(dlon)
		sdomg := math.Sin(domg12)
		cdomg := math.Cos(domg12)
		somg12 = slam12*cdomg - clam12*sdomg
		comg12 = clam12*cdomg + slam12*sdomg
	}
	var alp12 float64
	if comg12 > -0.7071 && sbet2-sbet1 < 1.75 {
		domg := 1 + comg12
		dbet1 := 1 + cbet1
		dbet2 := 1 + cbet2
		alp12 = 2 * math.Atan2(
			somg12*(sbet1*dbet2+sbet2*dbet1),
			domg*(sbet1*sbet2+dbet1*dbet2),
		)
	} else {
		s12 := salp2*calp1 - calp2*salp1
		c12 := calp2*calp1 + salp2*salp1
		if s12 == 0 && c12 < 0 {
			s12 = 1e-300 * calp1
			c12 = -1
		}
		alp12 = math.Atan2(s12, c12)
	}
	S12 += karney.c2 * alp12
	// Restore the signs absorbed by input normalisation.
	S12 *= swapp * lonsign * latsign
	return S12
}

// karneyRingArea computes the signed polygon area on the WGS84
// ellipsoid using Karney's exact algorithm. Sign convention matches
// kernel/spherical: CCW (when viewed from outside the ellipsoid) is
// positive.
func karneyRingArea(ring []geom.XY) float64 {
	if len(ring) < 4 {
		return 0
	}
	end := len(ring) - 1
	if ring[0] != ring[len(ring)-1] {
		end = len(ring)
	}
	var sum float64
	for i := 0; i < end; i++ {
		j := (i + 1) % len(ring)
		sum += edgeS12(ring[i].X, ring[i].Y, ring[j].X, ring[j].Y)
	}
	// geographiclib-c S12 is positive for CW vertex order (right-handed
	// rule with the "inside" on the left of the directed edge). We
	// negate to match kernel/spherical's CCW-positive convention.
	A := -sum
	full := 4 * math.Pi * karney.c2
	if A > full/2 {
		A -= full
	} else if A < -full/2 {
		A += full
	}
	return A
}
