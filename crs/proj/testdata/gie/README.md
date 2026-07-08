# Vendored PROJ test fixtures

The `.gie` files in this directory are taken verbatim from PROJ's own
regression-test corpus
([github.com/OSGeo/PROJ](https://github.com/OSGeo/PROJ/tree/master/test/gie)).
They are the same fixtures PROJ itself runs to verify projection
correctness; go-topology-suite uses them to validate its pure-Go reimplementations
of Mercator, Transverse Mercator, Lambert Conformal Conic, Albers
Equal-Area, and Lambert Azimuthal Equal-Area.

License: see `LICENSE` in this directory (Frank Warmerdam / OSGeo,
MIT-style).

## File format

Each `<gie>` or `<gie-strict>` block defines an `operation` (a Proj4
parameter string), one or more `accept` / `expect` coordinate pairs, and
an optional `tolerance`. `direction inverse` flips subsequent pairs to
exercise the inverse mapping. The parser in
`crs/proj/internal/gie/parser.go` understands this subset.
