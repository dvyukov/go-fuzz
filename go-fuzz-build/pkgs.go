// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// packageDir returns local directory with package source files.
func packageDir(p *packages.Package) string {
	// Go-package contains at least one go-file, so GoFiles is not empty without fail.
	dir := filepath.Dir(p.GoFiles[0])
	return dir
}

// isNotPackage checks if dir contains go source files.
func isNotPackage(files []os.FileInfo) bool {
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".go") {
			return false
		}
	}
	return true
}
