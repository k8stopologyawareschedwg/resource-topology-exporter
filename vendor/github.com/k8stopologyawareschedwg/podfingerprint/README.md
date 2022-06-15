[![Go Reference](https://pkg.go.dev/badge/github.com/k8stopologyawareschedwg/podfingerprint.svg)](https://pkg.go.dev/github.com/k8stopologyawareschedwg/podfingerprint)

# podfingerprint: compute the fingerprint of a set of pods

This package computes the fingerprint of a set of [kubernetes pods](https://kubernetes.io/docs/concepts/workloads/pods).
For the purposes of this package, a Pod is only its namespace + name pair, used to identify it.
A "fingerprint" is a compact unique representation of this set of pods.
Any given unordered set of pods with the same elements will yield the same fingerprint, regardless of the order on which the pods are enumerated.
The fingerprint is not actually unique because it is implemented using a hash function, but the collisions are expected to be extremely low.
Note this package will *NOT* restrict itself to use only cryptographically secure hash functions, so you should NOT use the fingerprint in security-sensitive contexts.

## LICENSE

apache v2

