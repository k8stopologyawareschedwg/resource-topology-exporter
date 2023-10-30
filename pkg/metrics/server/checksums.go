/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"os"

	"k8s.io/klog/v2"
)

type checksums struct {
	cert   []byte
	key    []byte
	authCA []byte
}

func (ck checksums) IsValid() bool {
	return len(ck.cert) > 0 && len(ck.key) > 0 && len(ck.authCA) > 0
}

func (ck checksums) Equal(ck2 checksums) bool {
	if !bytes.Equal(ck.cert, ck2.cert) {
		klog.Infof("cert changed: %x -> %x", ck.cert, ck2.cert)
		return false
	}
	if !bytes.Equal(ck.key, ck2.key) {
		klog.Infof("key changed: %x -> %x", ck.key, ck2.key)
		return false
	}
	if !bytes.Equal(ck.authCA, ck2.authCA) {
		klog.Infof("auth CA changed: %x -> %x", ck.authCA, ck2.authCA)
		return false
	}
	return true
}

func newChecksums() checksums {
	return newChecksumsWithPaths(tlsCert, tlsKey, authCAFile)
}

func allFilesReady() bool {
	return allFilesReadyWithPaths(tlsCert, tlsKey, authCAFile)
}

// newChecksumsWithPaths is meant for testing purposes
func newChecksumsWithPaths(tlsCertPath, tlsKeyPath, authCAFilePath string) checksums {
	return checksums{
		cert:   checksumFile(tlsCertPath),
		key:    checksumFile(tlsKeyPath),
		authCA: checksumFile(authCAFilePath),
	}
}

// allFilesReadyWithPaths is meant for testing purposes
func allFilesReadyWithPaths(tlsCertPath, tlsKeyPath, authCAFilePath string) bool {
	type entrySpec struct {
		path string
		desc string
	}

	entries := []entrySpec{
		{
			path: tlsCertPath,
			desc: "TLS cert",
		},
		{
			path: tlsKeyPath,
			desc: "TLS key",
		},
		{
			path: authCAFilePath,
			desc: "auth CA",
		},
	}

	for _, entry := range entries {
		ok, err := fileExistsAndNotEmpty(entry.path)
		if err != nil {
			klog.Warningf("error checking if changed %s file empty/exists: %v", entry.desc, err)
			return false
		}
		if !ok {
			klog.V(2).Infof("%s missing or empty, certificates will not be rotated", entry.desc)
			return false
		}
	}

	return true
}

func checksumFile(name string) []byte {
	file, err := os.Open(name)
	if err != nil {
		klog.Errorf("failed to open file %v for checksum: %v", name, err)
		return []byte{}
	}
	defer file.Close()

	hash := sha256.New()

	if _, err = io.Copy(hash, file); err != nil {
		klog.Errorf("failed to compute checksum for file %v: %v", name, err)
		return []byte{}
	}

	return hash.Sum(nil)
}

func fileExistsAndNotEmpty(name string) (bool, error) {
	fi, err := os.Stat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return (fi.Size() != 0), nil
}
