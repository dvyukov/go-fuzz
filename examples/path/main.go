// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package path

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
)

func Fuzz(data []byte) int {
	sdata := string(data)
	sdata0 := sdata[:len(sdata)/2]
	sdata1 := sdata[len(sdata)/2:]

	path.Base(sdata)
	cleaned := path.Clean(sdata)
	if cleaned2 := path.Clean(cleaned); cleaned != cleaned2 {
		fmt.Printf("was:      %q\n", sdata)
		fmt.Printf("cleaned:  %q\n", cleaned)
		fmt.Printf("cleaned2: %q\n", cleaned2)
		panic("path.Clean undercleans")
	}
	path.Dir(sdata)
	path.Ext(sdata)
	path.IsAbs(sdata)
	path.Clean(sdata)
	dir, file := path.Split(sdata)
	joined := path.Join(dir, file)
	if false && len(dir) != 0 && len(file) != 0 && joined != sdata {
		fmt.Printf("was: %q\n", sdata)
		fmt.Printf("now: %q (%q, %q)\n", joined, dir, file)
		panic("Split/Join changed path")
	}
	path.Match(sdata0, sdata1)

	isAbs := filepath.IsAbs(sdata)
	abs, err := filepath.Abs(sdata)
	if isAbs && err != nil /* isAbs && (err != nil || abs != sdata) || !isAbs && (err == nil && abs == sdata) */ {
		fmt.Printf("was: %q\n", sdata)
		fmt.Printf("isabs=%v abs=%q err=%v\n", isAbs, abs, err)
		panic("IsAbs lies")
	}
	filepath.Base(sdata)
	cleaned = filepath.Clean(sdata)
	if cleaned2 := filepath.Clean(cleaned); cleaned != cleaned2 {
		fmt.Printf("was:      %q\n", sdata)
		fmt.Printf("cleaned:  %q\n", cleaned)
		fmt.Printf("cleaned2: %q\n", cleaned2)
		panic("filepath.Clean undercleans")
	}
	filepath.EvalSymlinks(sdata)
	filepath.FromSlash(sdata)
	slashed := filepath.ToSlash(sdata)
	unslashed := filepath.FromSlash(slashed)
	if unslashed != sdata {
		panic("ToSlash/FromSlash corrupt path")
	}
	// filepath.Glob(sdata) can scan whole disk
	filepath.HasPrefix(sdata0, sdata1)
	filepath.VolumeName(sdata)
	filepath.Split(sdata)
	filepath.Join(sdata0, sdata1)
	parts := filepath.SplitList(sdata)
	joined = filepath.Join(parts...)
	parts1 := filepath.SplitList(sdata)
	if !reflect.DeepEqual(parts, parts1) {
		panic("Split/Join non-idempotent")
	}
	filepath.Rel(sdata0, sdata1)
	filepath.Match(sdata0, sdata1)
	return 0
}
