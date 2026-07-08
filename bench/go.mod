// Module bench holds the benchmark harness, the gts-bench CLI, and the
// cross-implementation conformance suite. It is a separate module so its
// dependencies (notably simplefeatures) stay out of the library's module
// graph; it is not intended to be imported.
module github.com/exergy-dev/go-topology-suite/bench

go 1.23

require (
	github.com/exergy-dev/go-topology-suite v0.1.0
	github.com/peterstace/simplefeatures v0.59.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/exergy-dev/go-topology-suite => ../
