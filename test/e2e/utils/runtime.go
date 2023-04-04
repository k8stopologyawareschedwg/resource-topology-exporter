package utils

import (
	"fmt"
	"path/filepath"
	"runtime"
)

var BinariesPath string
var TestDataPath string

func init() {

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Printf("Cannot retrieve tests directory")
	}

	baseDir := filepath.Dir(file)
	BinariesPath = filepath.Clean(filepath.Join(baseDir, "..", "..", "..", "./_out"))
	TestDataPath = filepath.Clean(filepath.Join(baseDir, "..", "..", "..", "test", "data"))
}
