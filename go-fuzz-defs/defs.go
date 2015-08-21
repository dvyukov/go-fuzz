// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package base

const (
	CoverSize       = 64 << 10
	MaxInputSize    = 1 << 20
	SonarRegionSize = 1 << 20
)

const (
	SonarEQL = iota
	SonarNEQ
	SonarLSS
	SonarGTR
	SonarLEQ
	SonarGEQ

	SonarOpMask = 7
	SonarLength = 1 << 3
	SonarSigned = 1 << 4
	SonarString = 1 << 5
	SonarConst1 = 1 << 6
	SonarConst2 = 1 << 7

	SonarHdrLen = 6
	SonarMaxLen = 20
)

type CoverBlock struct {
	ID        int
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmt   int
}

type Literal struct {
	Val   string
	IsStr bool
}

type MetaData struct {
	Literals []Literal
	Blocks   []CoverBlock
	Sonar    []CoverBlock
}
