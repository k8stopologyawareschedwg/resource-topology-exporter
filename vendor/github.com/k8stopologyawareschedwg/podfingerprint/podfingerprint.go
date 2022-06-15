/*
 * Copyright 2022 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package podfingerprint computes the fingerprint of a set of pods.
// "Pods" is meant in the kubernetes sense:
// https://kubernetes.io/docs/concepts/workloads/pods/
// but for the purposes of this package, a Pod is identified by just
// its namespace + name pair.
// A "fingerprint" is a compact unique representation of this set of pods.
// Any given unordered set of pods with the same elements will yield
// the same fingerprint, regardless of the order on which the pods are enumerated.
// The fingerprint is not actually unique because it is implemented
// using a hash function, but the collisions are expected to be extremely low.
// Note this package will *NOT* restrict itself to use only cryptographically
// secure hash functions, so you should NOT use the fingerprint in
// security-sensitive contexts.
package podfingerprint

import (
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/OneOfOne/xxhash"
)

const (
	// Prefix is the string common to all the fingerprints
	// A prefix is always 4 bytes long
	Prefix = "pfp0"
	// Version is the version of this fingerprint. You should
	// only compare compatible versions.
	// A Version is always 4 bytes long, in the form v\X\X\X
	Version = "v001"
	// Annotation is the recommended key to use to annotate objects
	// with the fingerprint.
	Annotation = "topology.node.k8s.io/fingerprint"
)

const (
	expectedPrefixLen  = 4
	expectedVersionLen = 4
	minimumSumLen      = 8
)

var (
	ErrMalformed           = fmt.Errorf("malformed fingerprint")
	ErrIncompatibleVersion = fmt.Errorf("incompatible version")
	ErrSignatureMismatch   = fmt.Errorf("fingerprint mismatch")
)

// IsVersionCompatible check if this package is compatible with the provided fingerprint version.
// Returns bool if the fingerprint version and the package are compatible, false otherwise.
// Returns error if the fingerprint version cannot be safely decoded.
func IsVersionCompatible(ver string) (bool, error) {
	if len(ver) != expectedVersionLen {
		return false, ErrMalformed
	}
	return ver == Version, nil
}

// PodIdentifier represent the minimal interface this package needs to identify a Pod
type PodIdentifier interface {
	GetNamespace() string
	GetName() string
}

// Fingerprint represent the fingerprint of a set of pods
type Fingerprint struct {
	hashes []uint64
}

// NewFingerprint creates a empty Fingerprint.
// The size parameter is a hint for the expected size of the pod set. Use 0 if you don't know.
// Values of size < 0 are ignored.
func NewFingerprint(size int) *Fingerprint {
	data := []uint64{}
	if size > 0 {
		data = make([]uint64, 0, size)
	}
	return &Fingerprint{hashes: data}
}

// AddPod adds a pod to the pod set.
func (fp *Fingerprint) AddPod(pod PodIdentifier) error {
	fp.addHash(pod.GetNamespace(), pod.GetName())
	return nil
}

// AddPod add a pod by its namespace/name pair to the pod set.
func (fp *Fingerprint) Add(namespace, name string) error {
	fp.addHash(namespace, name)
	return nil
}

// Sum computes the fingerprint of the *current* pod set as slice of bytes.
// It is legal to keep adding pods after calling Sum, but the fingerprint of
// the set will change. The output of Sum is guaranteed to be stable if the
// content of the pod set is stable.
func (fp *Fingerprint) Sum() []byte {
	sort.Sort(uvec64(fp.hashes))
	h := xxhash.New64()
	b := make([]byte, 8)
	for _, hash := range fp.hashes {
		h.Write(putUint64(b, hash))
	}
	return h.Sum(nil)
}

// Sign computes the pod set fingerprint as string.
// The string should be considered a opaque identifier and checked only for
// equality, or fed into Check
func (fp *Fingerprint) Sign() string {
	return Prefix + Version + hex.EncodeToString(fp.Sum())
}

// Check verifies if the provided fingerprint matches the current pod set.
// Returns nil if the verification holds, error otherwise, or if the fingerprint
// string is malformed.
func (fp *Fingerprint) Check(sign string) error {
	if len(sign) < expectedPrefixLen+expectedVersionLen+minimumSumLen {
		return ErrMalformed
	}
	pfx := sign[0:4]
	if pfx != Prefix {
		return ErrMalformed
	}
	ver := sign[4:8]
	ok, err := IsVersionCompatible(ver)
	if err != nil {
		return err
	}
	if !ok {
		return ErrIncompatibleVersion
	}
	sum := sign[8:]
	got := hex.EncodeToString(fp.Sum())
	if got != sum {
		return fmt.Errorf("%w got=%q expected=%q", ErrSignatureMismatch, got, sum)
	}
	return nil
}

// this is for speed not for code reuse. This helper can be inlined easily
func (fp *Fingerprint) addHash(namespace, name string) {
	fp.hashes = append(fp.hashes,
		xxhash.ChecksumString64S(
			name,
			xxhash.ChecksumString64(namespace),
		),
	)
}

type uvec64 []uint64

func (a uvec64) Len() int           { return len(a) }
func (a uvec64) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a uvec64) Less(i, j int) bool { return a[i] < a[j] }

func putUint64(b []byte, v uint64) []byte {
	_ = b[7] // early bounds check to guarantee safety of writes below
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
	return b
}
