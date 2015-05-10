package fmt

import (
	"fmt"
)

func Fuzz(data []byte) int {
	sdata := string(data)
	fmt.Sscanf("42", sdata)
	fmt.Sprintf(sdata)
	i := 0
	fmt.Sscanf("42", sdata, &i)
	fmt.Sprintf(sdata, i)
	f := 0.0
	fmt.Sscanf("42", sdata, &f)
	fmt.Sprintf(sdata, f)
	s := ""
	fmt.Sscanf("42", sdata, &s)
	fmt.Sprintf(sdata, s)
	x := struct{ X, Y int }{}
	fmt.Sscanf("42", sdata, &x)
	fmt.Sprintf(sdata, x)
	fmt.Sscanf(sdata, sdata, &s, &i)
	return 0
}
