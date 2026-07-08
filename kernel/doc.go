// Package kernel defines the strategy interface that supplies geometric
// primitives — orientation, segment intersection, distance, point-in-ring,
// and so on — to every higher-level operation.
//
// There are exactly three implementations, each in its own subpackage:
//
//   - kernel/planar    Cartesian; the textbook 2D plane.
//   - kernel/spherical Treats Earth as a sphere; lon/lat input.
//   - kernel/geodesic  WGS84 ellipsoid; Karney's algorithm.
//
// Operations select the kernel via a functional Option (predicate.WithKernel
// etc.); when omitted the default is inferred from the geometry's CRS:
// geographic CRSes route to spherical, everything else to planar.
//
// Third-party implementations are possible: the interface is public and
// kernels are passed by value, so a custom kernel.Kernel can be injected
// per call via the same options.
package kernel
