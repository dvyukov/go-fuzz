package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

type PersistentSet struct {
	dir string
	m   map[Sig]Artifact
}

type Artifact struct {
	data []byte
	meta uint64
}

type Sig [sha1.Size]byte

func hash(data []byte) Sig {
	return Sig(sha1.Sum(data))
}

func newPersistentSet(dir string) *PersistentSet {
	ps := &PersistentSet{
		dir: dir,
		m:   make(map[Sig]Artifact),
	}
	os.MkdirAll(dir, 0770)
	ps.readInDir(dir)
	return ps
}

func (ps *PersistentSet) readInDir(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("error during dir walk: %v\n", err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("error during file read: %v\n", err)
			return nil
		}
		sig := hash(data)
		if _, ok := ps.m[sig]; ok {
			return nil
		}
		name := info.Name()
		if len(name) > sha1.Size+1 && name[sha1.Size] == '.' {
			return nil // description file
		}
		var meta uint64
		if len(name) > sha1.Size+1 && name[sha1.Size] == '-' {
			meta, _ = strconv.ParseUint(name[sha1.Size+1:], 10, 64)
		}
		a := Artifact{data, meta}
		ps.m[sig] = a
		return nil
	})
}

func (ps *PersistentSet) add(a Artifact) bool {
	sig := hash(a.data)
	if _, ok := ps.m[sig]; ok {
		return false
	}
	ps.m[sig] = a
	fname := filepath.Join(ps.dir, hex.EncodeToString(sig[:]))
	if a.meta != 0 {
		fname += fmt.Sprintf("-%v", a.meta)
	}
	if err := ioutil.WriteFile(fname, a.data, 0660); err != nil {
		log.Printf("failed to write file: %v", err)
	}
	return true
}

func (ps *PersistentSet) addDescription(data []byte, desc []byte, typ string) {
	sig := hash(data)
	fname := filepath.Join(ps.dir, fmt.Sprintf("%v.%v", hex.EncodeToString(sig[:]), typ))
	if err := ioutil.WriteFile(fname, desc, 0660); err != nil {
		log.Printf("failed to write file: %v", err)
	}
}
