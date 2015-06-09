package fuzz

import (
	"math"
	"reflect"
)

// DeepEqual is reflect.DeepEqual except that:
// 1. nil and empty slice/string are considered equal
// 2. NaNs compare equal.
func DeepEqual(a1, a2 interface{}) bool {
	if a1 == nil || a2 == nil {
		return a1 == a2
	}
	return deepValueEqual(reflect.ValueOf(a1), reflect.ValueOf(a2), make(map[visit]bool))
}

func deepValueEqual(v1, v2 reflect.Value, visited map[visit]bool) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}

	hard := func(k reflect.Kind) bool {
		switch k {
		case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct:
			return true
		}
		return false
	}

	if v1.CanAddr() && v2.CanAddr() && hard(v1.Kind()) {
		addr1 := v1.UnsafeAddr()
		addr2 := v2.UnsafeAddr()
		if addr1 > addr2 {
			// Canonicalize order to reduce number of entries in visited.
			addr1, addr2 = addr2, addr1
		}

		// Short circuit if references are identical ...
		if addr1 == addr2 {
			return true
		}

		// ... or already seen
		typ := v1.Type()
		v := visit{addr1, addr2, typ}
		if visited[v] {
			return true
		}

		// Remember for later.
		visited[v] = true
	}

	switch v1.Kind() {
	case reflect.Array:
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.Pointer() == v2.Pointer() {
			return true
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case reflect.Ptr:
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case reflect.Struct:
		for i, n := 0, v1.NumField(); i < n; i++ {
			if !deepValueEqual(v1.Field(i), v2.Field(i), visited) {
				return false
			}
		}
		return true
	case reflect.Map:
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.Pointer() == v2.Pointer() {
			return true
		}
		for _, k := range v1.MapKeys() {
			if !deepValueEqual(v1.MapIndex(k), v2.MapIndex(k), visited) {
				return false
			}
		}
		return true
	case reflect.Func:
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		// Can't do better than this:
		return false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v1.Int() == v2.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v1.Uint() == v2.Uint()
	case reflect.Float32, reflect.Float64:
		f1 := v1.Float()
		f2 := v2.Float()
		return f1 == f2 || math.IsNaN(f1) && math.IsNaN(f2)
	case reflect.Complex64, reflect.Complex128:
		c1 := v1.Complex()
		c2 := v2.Complex()
		r1, i1 := real(c1), imag(c1)
		r2, i2 := real(c2), imag(c2)
		return (r1 == r2 || math.IsNaN(r1) && math.IsNaN(r2)) && (i1 == i2 || math.IsNaN(i1) && math.IsNaN(i2))
	case reflect.String:
		return v1.String() == v2.String()
	case reflect.UnsafePointer:
		return v1.Pointer() == v2.Pointer()
	case reflect.Bool:
		return v1.Bool() == v2.Bool()
	case reflect.Chan:
		return true
	default:
		panic("can't happen")
	}
}

type visit struct {
	a1  uintptr
	a2  uintptr
	typ reflect.Type
}
