package utils

import (
	"fmt"
	"path/filepath"
	"runtime"
)

var BinariesPath string

func init() {

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Printf("Cannot retrieve tests directory")
	}

	baseDir := filepath.Dir(file)
	BinariesPath = filepath.Clean(filepath.Join(baseDir, "..", "..", "..", "./_out"))
}
