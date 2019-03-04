// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// Package versifier recognizes internal structure of random (currently text-only) data
// and allows to generate data of a similar structure (for some very weak definition of "similar").
package versifier

/*
On efficiency of versifier.
On xml text after 2.5 hours of fuzzing:
Without versifier fuzzing discovered 902 inputs.
With versifier fuzzing discovered 1055 inputs and versifier discovered 83 inputs.
Versifier generated new inputs + increased fuzzing efficiency by 25% +
uncovered 62 new basic blocks (excluding counters) which accounts for 2.5% of all discovered basic blocks.
On json test after 1 hour of fuzzing:
Versifier uncovered 15 new basic blocks (excluding counters) which accounts for 1.15% of all discovered basic blocks.

Research on automatic protocol reverse engineering:
- Sequitur (or Nevill-Manning algorithm) algorithm:
http://en.wikipedia.org/wiki/Sequitur_algorithm
- Discoverer: Automatic Protocol Reverse Engineering from Network Traces
http://research.microsoft.com/pubs/153196/discoverer-security07.pdf
- Reverse Engineering of Protocols from Network Traces
http://www.di.fc.ul.pt/~nuno/PAPERS/WCRE11.pdf
*/

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/dvyukov/go-fuzz/go-fuzz/internal/pcg"
)

func BuildVerse(oldv *Verse, data []byte) *Verse {
	// Check if the data is something texty. If not, don't bother parsing it.
	// Versifier don't know how to recognize structure in binary data.
	// TODO: we could detect detect text and binary parts and handle them separately
	// (think of an HTTP request with compressed body).
	printable := 0
	for _, b := range data {
		if b >= 0x20 && b < 0x7f {
			printable++
		}
	}
	if printable < len(data)*9/10 {
		return oldv
	}

	newv := &Verse{}
	if oldv != nil {
		newv.blocks = oldv.blocks
		newv.allNodes = oldv.allNodes
	}
	n := tokenize(data)
	n = structure(n)
	b := &BlockNode{n}
	newv.blocks = append(newv.blocks, b)
	b.Visit(func(n Node) {
		newv.allNodes = append(newv.allNodes, n)
	})
	newv.r = pcg.New()
	return newv
}

type Node interface {
	Visit(f func(Node))
	Print(w io.Writer, ident int)
	Generate(w io.Writer, v *Verse)
}

func makeDict(s []byte) map[string]struct{} {
	return map[string]struct{}{string(s): struct{}{}}
}

func fmtDict(dict map[string]struct{}) string {
	var list []string
	for s := range dict {
		list = append(list, fmt.Sprintf("%q", s))
	}
	return strings.Join(list, ",")
}

func randTerm(v *Verse, dict map[string]struct{}) []byte {
	terms := make([]string, 0, len(dict))
	for k := range dict {
		terms = append(terms, k)
	}
	return []byte(terms[v.Rand(len(terms))])
}

func singleTerm(dict map[string]struct{}) string {
	for k := range dict {
		return k
	}
	panic("bad")
}

type WsNode struct {
	dict map[string]struct{}
}

func (n *WsNode) Visit(f func(n Node)) {
	f(n)
}

func (n *WsNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%sws{%s}\n", strings.Repeat("  ", ident), fmtDict(n.dict))
}

func (n *WsNode) Generate(w io.Writer, v *Verse) {
	if v.Rand(5) != 0 {
		w.Write(randTerm(v, n.dict))
	} else {
	loop:
		for {
			switch v.Rand(3) {
			case 0:
				break loop
			case 1:
				w.Write([]byte{' '})
			case 2:
				w.Write([]byte{'\t'})
			}
		}
	}
}

type AlphaNumNode struct {
	dict map[string]struct{}
}

func (n *AlphaNumNode) Visit(f func(n Node)) {
	f(n)
}

func (n *AlphaNumNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%salphanum{%s}\n", strings.Repeat("  ", ident), fmtDict(n.dict))
}

func (n *AlphaNumNode) Generate(w io.Writer, v *Verse) {
	if v.Rand(5) != 0 {
		w.Write(randTerm(v, n.dict))
	} else {
		len := 0
		switch v.Rand(3) {
		case 0:
			len = v.Rand(4)
		case 1:
			len = v.Rand(20)
		case 2:
			len = v.Rand(100)
		}
		res := make([]byte, len)
		for i := range res {
			switch v.Rand(4) {
			case 0:
				res[i] = '_'
			case 1:
				res[i] = '0' + byte(v.Rand(10))
			case 2:
				res[i] = 'a' + byte(v.Rand(26))
			case 3:
				res[i] = 'A' + byte(v.Rand(26))
			}
		}
		w.Write(res)
	}
}

type NumNode struct {
	dict map[string]struct{}
	hex  bool
}

func (n *NumNode) Visit(f func(n Node)) {
	f(n)
}

func (n *NumNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%snum{hex=%v, %s}\n", strings.Repeat("  ", ident), n.hex, fmtDict(n.dict))
}

func (n *NumNode) Generate(w io.Writer, v *Verse) {
	if v.Rand(2) == 0 {
		w.Write(randTerm(v, n.dict))
	} else {
		randNum := func() []byte {
			base := []int{8, 10, 16}[v.Rand(3)]
			len := 0
			switch v.Rand(3) {
			case 0:
				len = v.Rand(4)
			case 1:
				len = v.Rand(16)
			case 2:
				len = v.Rand(40)
			}
			num := make([]byte, len+1)
			for i := range num {
				switch base {
				case 8:
					num[i] = '0' + byte(v.Rand(8))
				case 10:
					num[i] = '0' + byte(v.Rand(10))
				case 16:
					switch v.Rand(3) {
					case 0:
						num[i] = '0' + byte(v.Rand(10))
					case 1:
						num[i] = 'a' + byte(v.Rand(6))
					case 2:
						num[i] = 'A' + byte(v.Rand(6))
					}
				}
			}
			switch base {
			case 8:
				num = append([]byte{'0'}, num...)
			case 10:
			case 16:
				num = append([]byte{'0', 'x'}, num...)
			default:
				panic("bad")
			}
			if v.Rand(2) == 0 {
				num = append([]byte{'-'}, num...)
			}
			return num
		}
		switch v.Rand(3) {
		case 0:
			w.Write(randNum())
		case 1:
			w.Write(randNum())
			w.Write([]byte{'.'})
			w.Write(randNum())
		case 2:
			w.Write(randNum())
			w.Write([]byte{'e'})
			w.Write(randNum())
		}
	}
}

type ControlNode struct {
	ch rune
}

func (n *ControlNode) Visit(f func(n Node)) {
	f(n)
}

func (n *ControlNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%s%q\n", strings.Repeat("  ", ident), string(n.ch))
}

func (n *ControlNode) Generate(w io.Writer, v *Verse) {
	if v.Rand(10) != 0 {
		w.Write([]byte{byte(n.ch)})
	} else {
		for {
			b := byte(v.Rand(128))
			if b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' {
				continue
			}
			w.Write([]byte{b})
			break
		}
	}
}

type BracketNode struct {
	open rune
	clos rune
	b    *BlockNode
}

var brackets = map[rune]rune{
	'<':  '>',
	'[':  ']',
	'(':  ')',
	'{':  '}',
	'\'': '\'',
	'"':  '"',
	'`':  '`',
}

func (n *BracketNode) Visit(f func(n Node)) {
	f(n)
	n.b.Visit(f)
}

func (n *BracketNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", ident), string(n.open))
	n.b.Print(w, ident+1)
	fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", ident), string(n.clos))
}

func (n *BracketNode) Generate(w io.Writer, v *Verse) {
	if v.Rand(10) != 0 {
		w.Write([]byte{byte(n.open)})
		n.b.Generate(w, v)
		w.Write([]byte{byte(n.clos)})
	} else {
		brk := []rune{'<', '[', '(', '{', '\'', '"', '`'}
		open := brk[v.Rand(len(brk))]
		clos := brackets[open]
		if v.Rand(5) == 0 {
			clos = brackets[brk[v.Rand(len(brk))]]
		}
		w.Write([]byte{byte(open)})
		n.b.Generate(w, v)
		w.Write([]byte{byte(clos)})
	}
}

type KeyValNode struct {
	delim rune
	key   *AlphaNumNode
	value *AlphaNumNode
}

func (n *KeyValNode) Visit(f func(n Node)) {
	f(n)
	n.key.Visit(f)
	n.value.Visit(f)
}
func (n *KeyValNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%skeyval\n", strings.Repeat("  ", ident))
	n.key.Print(w, ident+1)
	fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", ident+1), string(n.delim))
	n.value.Print(w, ident+1)
}

func (n *KeyValNode) Generate(w io.Writer, v *Verse) {
	delim := []rune{'=', ':'}
	n.delim = delim[v.Rand(len(delim))]
	n.key.Generate(w, v)
	w.Write([]byte{byte(n.delim)})
	n.value.Generate(w, v)
}

type ListNode struct {
	delim  rune
	blocks []*BlockNode
}

func (n *ListNode) Visit(f func(n Node)) {
	f(n)
	for _, b := range n.blocks {
		b.Visit(f)
	}
}

func (n *ListNode) Print(w io.Writer, ident int) {
	fmt.Fprintf(w, "%slist\n", strings.Repeat("  ", ident))
	for i, b := range n.blocks {
		if i != 0 {
			fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", ident), string(n.delim))
		}
		b.Print(w, ident+1)
	}
}

func (n *ListNode) Generate(w io.Writer, v *Verse) {
	blocks := n.blocks
	if v.Rand(5) == 0 {
		blocks = nil
		for v.Rand(3) != 0 {
			blocks = append(blocks, n.blocks[v.Rand(len(n.blocks))])
		}
	}
	for i, b := range blocks {
		if i != 0 {
			w.Write([]byte{byte(n.delim)})
		}
		b.Generate(w, v)
	}
}

type LineNode struct {
	r bool
	b *BlockNode
}

func (n *LineNode) Visit(f func(n Node)) {
	f(n)
	n.b.Visit(f)
}

func (n *LineNode) Print(w io.Writer, ident int) {
	rn := "\\n"
	if n.r {
		rn = "\\r\\n"
	}
	fmt.Fprintf(w, "%sline %s\n", strings.Repeat("  ", ident), rn)
	n.b.Print(w, ident+1)
}

func (n *LineNode) Generate(w io.Writer, v *Verse) {
	n.b.Generate(w, v)
	if n.r {
		w.Write([]byte{'\r', '\n'})
	} else {
		w.Write([]byte{'\n'})
	}
}

type BlockNode struct {
	nodes []Node
}

func (n *BlockNode) Visit(f func(n Node)) {
	f(n)
	for _, n := range n.nodes {
		n.Visit(f)
	}
}

func (n *BlockNode) Print(w io.Writer, ident int) {
	for _, n := range n.nodes {
		n.Print(w, ident)
	}
}

func (n *BlockNode) Generate(w io.Writer, v *Verse) {
	nodes := append([]Node{}, n.nodes...)
	if v.Rand(10) == 0 {
		for len(nodes) > 0 && v.Rand(2) == 0 {
			idx := v.Rand(len(nodes))
			copy(nodes[:idx], nodes[idx+1:])
			nodes = nodes[:len(nodes)-1]
		}
	}
	if v.Rand(10) == 0 {
		for len(nodes) > 0 && v.Rand(2) == 0 {
			idx := v.Rand(len(nodes))
			nodes = append(nodes, nil)
			copy(nodes[idx+1:], nodes[idx:])
		}
	}
	if v.Rand(10) == 0 {
		for len(nodes) > 0 && v.Rand(2) == 0 {
			idx1 := v.Rand(len(nodes))
			idx2 := v.Rand(len(nodes))
			nodes[idx1], nodes[idx2] = nodes[idx2], nodes[idx1]
		}
	}
	for _, n := range nodes {
		if v.Rand(20) == 0 {
			continue
		}
		if v.Rand(20) == 0 {
			// TODO: replace subranges of nodes with other subranges of nodes.
			// That is, currently RandNode returns either a BlockNode or
			// an individual node within that BlockNode, but it ought to
			// be able to return a subrange of nodes within a BlockNode.
			n = v.RandNode()
		}
		n.Generate(w, v)
	}
}

type Verse struct {
	blocks   []*BlockNode
	allNodes []Node
	r        *pcg.Rand
}

func (v *Verse) Print(w io.Writer) {
	for _, b := range v.blocks {
		b.Print(w, 0)
		fmt.Fprintf(w, "========\n")
	}
}

func (v *Verse) Rhyme() []byte {
	buf := &bytes.Buffer{}
	v.blocks[v.Rand(len(v.blocks))].Generate(buf, v)
	return buf.Bytes()
}

func (v *Verse) Rand(n int) int {
	return v.r.Intn(n)
}

func (v *Verse) RandNode() Node {
	return v.allNodes[v.Rand(len(v.allNodes))]
}

func tokenize(data []byte) []Node {
	var res []Node
	const (
		stateControl = iota
		stateWs
		stateAlpha
		stateNum
	)
	state := stateControl
	start := 0
	for i := 0; i < len(data); {
		r, s := utf8.DecodeRune(data[i:])
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_':
			switch state {
			case stateControl:
				start = i
				state = stateAlpha
			case stateWs:
				res = append(res, &WsNode{makeDict(data[start:i])})
				start = i
				state = stateAlpha
			case stateAlpha:
			case stateNum:
				state = stateAlpha
			}

		case r >= '0' && r <= '9':
			switch state {
			case stateControl:
				start = i
				state = stateNum
			case stateWs:
				res = append(res, &WsNode{makeDict(data[start:i])})
				start = i
				state = stateNum
			case stateAlpha:
			case stateNum:
			}

		case r == ' ' || r == '\t':
			switch state {
			case stateControl:
				start = i
				state = stateWs
			case stateWs:
			case stateAlpha:
				res = append(res, &AlphaNumNode{makeDict(data[start:i])})
				start = i
				state = stateWs
			case stateNum:
				res = append(res, &NumNode{dict: makeDict(data[start:i])})
				start = i
				state = stateWs
			}

		default:
			switch state {
			case stateControl:
			case stateWs:
				res = append(res, &WsNode{makeDict(data[start:i])})
			case stateAlpha:
				res = append(res, &AlphaNumNode{makeDict(data[start:i])})
			case stateNum:
				res = append(res, &NumNode{dict: makeDict(data[start:i])})
			}
			state = stateControl
			res = append(res, &ControlNode{r})
		}
		i += s
	}
	switch state {
	case stateAlpha:
		res = append(res, &AlphaNumNode{map[string]struct{}{string(data[start:]): struct{}{}}})
	case stateNum:
		res = append(res, &NumNode{dict: map[string]struct{}{string(data[start:]): struct{}{}}})
	}
	return res
}

func structure(nn []Node) []Node {
	nn = extractNumbers(nn)
	nn = structureBrackets(nn)
	nn = structureKeyValue(nn)
	nn = structureLists(nn)
	nn = structureLines(nn)
	return nn
}

func isHexNum(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			continue
		}
		return false
	}
	return true
}

func isDecNum(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func extractNumbers(nn []Node) []Node {
	// TODO: replace this mess with a real parser.
	for changed := true; changed; {
		changed = false
		for i := 0; i < len(nn); i++ {
			n := nn[i]
			if num, ok := n.(*AlphaNumNode); ok {
				v := singleTerm(num.dict)
				if len(v) >= 3 {
					if v[0] == '0' && v[1] == 'x' && isHexNum(v[2:]) {
						nn[i] = &NumNode{hex: true, dict: num.dict}
						changed = true
						continue
					}
					if e := strings.IndexByte(v, 'e'); e != -1 {
						if isDecNum(v[:e]) && isDecNum(v[e+1:]) {
							nn[i] = &NumNode{hex: false, dict: num.dict}
							changed = true
							continue
						}
						if e == len(v)-1 && i != len(nn)-1 {
							if num1, ok := nn[i+1].(*NumNode); ok {
								nn[i+1] = &NumNode{hex: false, dict: makeDict([]byte(v + singleTerm(num1.dict)))}
								copy(nn[i:], nn[i+1:])
								nn = nn[:len(nn)-1]
								changed = true
								continue
							}
						}
					}
				}
			}
			if minus, ok := n.(*ControlNode); ok && minus.ch == '-' && i != len(nn)-1 {
				if num, ok := nn[i+1].(*NumNode); ok {
					var prev Node
					if i != 0 {
						prev = nn[i-1]
					}
					// TODO: check that previous is not alphanum
					// e.g. ID-001, but allow 1e-1.
					if prev1, ok := prev.(*AlphaNumNode); !ok || len(singleTerm(prev1.dict)) > 1 && singleTerm(prev1.dict)[len(singleTerm(prev1.dict))-1] == 'e' {
						num.dict = makeDict([]byte("-" + singleTerm(num.dict)))
						copy(nn[i:], nn[i+1:])
						nn = nn[:len(nn)-1]
						changed = true
						continue
					}
				}
			}
			if ctrl, ok := n.(*ControlNode); ok && ctrl.ch == '.' && i != 0 && i != len(nn)-1 {
				num1, ok1 := nn[i-1].(*NumNode)
				num2, ok2 := nn[i+1].(*NumNode)
				if ok1 && ok2 {
					nn[i+1] = &NumNode{hex: false, dict: makeDict([]byte(singleTerm(num1.dict) + "." + singleTerm(num2.dict)))}
					copy(nn[i-1:], nn[i+1:])
					nn = nn[:len(nn)-2]
					changed = true
					continue
				}
			}
		}
	}
	return nn
}

func structureKeyValue(nn []Node) (res []Node) {
	// TODO: extract numeric key-value pairs
	delims := map[rune]bool{'=': true, ':': true}
	for _, n := range nn {
		if brk, ok := n.(*BracketNode); ok {
			brk.b.nodes = structureKeyValue(brk.b.nodes)
		}
	}

	for i := 0; i < len(nn); i++ {
		n := nn[i]
		ctrl, ok := n.(*ControlNode)
		if !ok {
			continue
		}
		if delims[ctrl.ch] &&
			!(i == 0 || i == len(nn)-1) {
			var key, value *AlphaNumNode
			key, ok = nn[i-1].(*AlphaNumNode)
			if !ok {
				continue
			}
			value, ok = nn[i+1].(*AlphaNumNode)
			if !ok {
				continue
			}
			nn[i+1] = &KeyValNode{ctrl.ch, key, value}
			copy(nn[i-1:], nn[i+1:])
			nn = nn[:len(nn)-2]
		}
	}
	return nn
}

func structureBrackets(nn []Node) []Node {
	type Brk struct {
		open rune
		clos rune
		pos  int
	}
	var stk []Brk
loop:
	for i := 0; i < len(nn); i++ {
		n := nn[i]
		ctrl, ok := n.(*ControlNode)
		if !ok {
			continue
		}
		for si := len(stk) - 1; si >= 0; si-- {
			if ctrl.ch == stk[si].clos {
				b := &BracketNode{stk[si].open, stk[si].clos, &BlockNode{append([]Node{}, nn[stk[si].pos+1:i]...)}}
				nn[stk[si].pos] = b
				copy(nn[stk[si].pos+1:], nn[i+1:])
				nn = nn[:len(nn)-i+stk[si].pos]
				i = stk[si].pos
				stk = stk[:si]
				continue loop
			}
		}
		if clos, ok := brackets[ctrl.ch]; ok {
			stk = append(stk, Brk{ctrl.ch, clos, i})
		}
	}
	return nn
}

func structureLists(nn []Node) (res []Node) {
	delims := map[rune]bool{',': true, ';': true}
	for _, n := range nn {
		if brk, ok := n.(*BracketNode); ok {
			brk.b.nodes = structureLists(brk.b.nodes)
		}
	}
	// TODO: fails on:
	//	"f1": "v1", "f2": "v2", "f3": "v3"
	// the first detected list is "v2", "f3"
	for i := len(nn) - 1; i >= 0; i-- {
		n := nn[i]
		if ctrl, ok := n.(*ControlNode); ok && delims[ctrl.ch] {
			type Elem struct {
				tok  map[rune]bool
				done bool
				pos  int
				inc  int
			}
			elems := [2]*Elem{
				{make(map[rune]bool), false, i - 1, -1},
				{make(map[rune]bool), false, i + 1, +1},
			}
			for {
				for _, e := range elems {
					if e.done || e.pos < 0 || e.pos >= len(nn) {
						e.done = true
						continue
					}
					if ctrl1, ok := nn[e.pos].(*ControlNode); ok {
						if ctrl1.ch == ctrl.ch {
							e.done = true
							continue
						}
						e.tok[ctrl1.ch] = true
					}
					if brk1, ok := nn[e.pos].(*BracketNode); ok {
						e.tok[brk1.open] = true
						e.tok[brk1.clos] = true
					}
					e.pos += e.inc
				}
				if elems[0].done && elems[1].done {
					break
				}
				union := make(map[rune]bool)
				for k := range elems[0].tok {
					union[k] = true
				}
				for k := range elems[1].tok {
					union[k] = true
				}
				if reflect.DeepEqual(elems[0].tok, union) || reflect.DeepEqual(elems[1].tok, union) {
					break
				}
			}

			for k := range elems[1].tok {
				elems[0].tok[k] = true
			}

		elemLoop:
			for _, e := range elems {
				for ; e.pos >= 0 && e.pos < len(nn); e.pos += e.inc {
					if ctrl1, ok := nn[e.pos].(*ControlNode); ok && !elems[0].tok[ctrl1.ch] {
						continue elemLoop
					}
					if brk1, ok := nn[e.pos].(*BracketNode); ok && !(elems[0].tok[brk1.open] && elems[0].tok[brk1.clos]) {
						continue elemLoop
					}
				}
			}

			for _, e := range elems {
				for {
					if e.done || e.pos < 0 || e.pos >= len(nn) {
						break
					}
					if ctrl1, ok := nn[e.pos].(*ControlNode); ok {
						if ctrl1.ch == ctrl.ch {
							break
						}
						if !elems[0].tok[ctrl1.ch] {
							break
						}
					}
					if brk1, ok := nn[e.pos].(*BracketNode); ok {
						if !elems[0].tok[brk1.open] || !elems[0].tok[brk1.clos] {
							break
						}
					}
					e.pos += e.inc
				}
			}
			lst := &ListNode{ctrl.ch, []*BlockNode{
				{append([]Node{}, nn[elems[0].pos+1:i]...)},
				{append([]Node{}, nn[i+1:elems[1].pos]...)},
			}}
			start := elems[0].pos
			end := elems[1].pos
			for {
				if start < 0 {
					break
				}
				if ctrl1, ok := nn[start].(*ControlNode); !ok || ctrl1.ch != ctrl.ch {
					break
				}
				pos := start - 1
				for {
					if pos < 0 {
						break
					}
					if ctrl1, ok := nn[pos].(*ControlNode); ok {
						if ctrl1.ch == ctrl.ch {
							break
						}
						if !elems[0].tok[ctrl1.ch] {
							break
						}
					}
					if brk1, ok := nn[pos].(*BracketNode); ok {
						if !elems[0].tok[brk1.open] || !elems[0].tok[brk1.clos] {
							break
						}
					}
					pos--
				}
				lst.blocks = append([]*BlockNode{{append([]Node{}, nn[pos+1:start]...)}}, lst.blocks...)
				start = pos
			}
			nn[start+1] = lst
			copy(nn[start+2:], nn[end:])
			nn = nn[:len(nn)-end+start+2]
			i = start + 1
		}
	}
	return nn
}

type NodeSet struct {
	ctrl map[rune]bool
	brk  map[rune]bool
}

func structureLines(nn []Node) (res []Node) {
	for i := 0; i < len(nn); i++ {
		n := nn[i]
		if brk, ok := n.(*BracketNode); ok {
			brk.b.nodes = structureLines(brk.b.nodes)
			continue
		}
		if ctrl, ok := n.(*ControlNode); !ok || ctrl.ch != '\n' {
			continue
		}
		r := false
		end := i
		if i != 0 {
			if prev, ok := nn[i-1].(*ControlNode); ok && prev.ch == '\r' {
				r = true
				end--
			}
		}
		res = append(res, &LineNode{r, &BlockNode{nn[:end]}})
		nn = nn[i+1:]
		i = -1
	}
	if len(nn) != 0 {
		res = append(res, nn...)
	}
	return res
}
