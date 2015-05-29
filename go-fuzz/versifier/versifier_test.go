package versifier

import (
	"os"
	"testing"
)

func dump(data string) {
	v := BuildVerse(nil, []byte(data))
	v.Print(os.Stdout)
}

func TestNumber(t *testing.T) {
	dump(`abc -10 def 0xab1 0x123 1e10 asd 1e2 22e-78 -11e72`)
}

func TestList1(t *testing.T) {
	dump(`{"f1": "v1", "f2": "v2", "f3": "v3"}`)
}

func TestList2(t *testing.T) {
	dump(`1,2.0,3e3`)
}
