package htmltemplate

import (
	"errors"
	"html/template"
	"io/ioutil"
	"regexp"
)

// Huge padding
var fmtHang = regexp.MustCompile("%[-+# 0]*(((([0-9]{3,})|[*]))|([0-9]*\\.(([0-9]{3,})|[*])))")

func Fuzz(data []byte) int {
	if fmtHang.Match(data) {
		return 0
	}
	t, err := template.New("foo").Funcs(funcs).Parse(string(data))
	if err != nil {
		if t != nil {
			panic("non nil template on error")
		}
		return 0
	}
	d := &Data{
		A: 42,
		B: "foo",
		C: []int{1, 2, 3},
		D: map[int]string{1: "foo", 2: "bar"},
		E: Data1{42, "foo"},
	}
	defer func() {
		x := recover()
		if x != nil {
			if str, ok := x.(string); ok && str == "unidentified node type in allIdents" {
				// https://github.com/golang/go/issues/11356
				return
			}
			panic(x)
		}
	}()
	t.Execute(ioutil.Discard, d)
	return 1
}

type Data struct {
	A int
	B string
	C []int
	D map[int]string
	E Data1
}

type Data1 struct {
	A int
	B string
}

func (Data1) Q() string {
	return "foo"
}

func (Data1) W() (string, error) {
	return "foo", nil
}

func (Data1) E() (string, error) {
	return "foo", errors.New("Data.E error")
}

func (Data1) R(v int) (string, error) {
	return "foo", nil
}

func (Data1) T(s string) (string, error) {
	return s, nil
}

var funcs = map[string]interface{}{
	"Q": func1,
	"W": func2,
	"E": func3,
	"R": func4,
	"T": func5,
	"Y": func6,
	"U": func7,
	"I": func8,
}

func func1(s string) string {
	return s
}

func func2(s string) (string, error) {
	return s, nil
}

func func3(s string) (string, error) {
	return s, errors.New("func3 error")
}

func func4(v int) int {
	return v
}

func func5(v int) (int, error) {
	return v, nil
}

func func6() int {
	return 42
}

func func7() (int, error) {
	return 42, nil
}

func func8() (int, error) {
	return 42, errors.New("func8 error")
}
