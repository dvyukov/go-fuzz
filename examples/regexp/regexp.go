package regexp

// int RE2Match(const char* restr, int restrlen, const char* str, int strlen, int* matched, char** error);
import "C"

import (
	//"fmt"
	"regexp"
	//"strings"
	//"unsafe"
)

/*
func RE2Match(restr, str []byte) (ok, matched bool, err string) {
	var rep, strp *C.char
	relen, strlen := C.int(len(restr)), C.int(len(str))
	if relen != 0 {
		rep = (*C.char)(unsafe.Pointer(&restr[0]))
	}
	if strlen != 0 {
		strp = (*C.char)(unsafe.Pointer(&str[0]))
	}
	var re2matched C.int
	var re2err *C.char
	re2ok := C.RE2Match(rep, relen, strp, strlen, &re2matched, &re2err)
	if re2ok == 0 {
		var b []byte
		raw := (*[1<<12]byte)(unsafe.Pointer(re2err))
		for _, c := range raw {
			if c == 0 {
				break
			}
			b = append(b, c)
		}
		err = string(b)
	}
	return re2ok != 0, re2matched != 0, err
}

func isAscii(b []byte) bool {
	for _, v := range b {
		if v == 0 || v >= 128 {
			return false
		}
	}
	return true
}
*/

func Fuzz(data []byte) int {
	str := data[:len(data)/2]
	sstr := string(str)
	//restrb := data[len(data)/2:]
	restr := string(data[len(data)/2:])

	quoted := regexp.QuoteMeta(sstr)
	req, err := regexp.Compile(quoted)
	if err == nil {
		if !req.MatchString(sstr) {
			panic("quoted is not matched")
		}
	}

	/*
	if isAscii(restrb) && isAscii(str) {
		re2ok, re2matched, re2err := RE2Match(restrb, str)
		re, err := regexp.Compile(restr)
		if (err == nil) != re2ok {
			if !(re2ok && (strings.HasPrefix(err.Error(), "error parsing regexp: invalid UTF-8") ||
				strings.HasPrefix(err.Error(), "error parsing regexp: invalid repeat count") ||
				strings.HasPrefix(err.Error(), "error parsing regexp: invalid escape sequence: `\\C`"))) {
				fmt.Printf("re=%q regexp=%v re2=%v(%v)\n", restr, err, re2ok, re2err)
				panic("regexp and re2 disagree on regexp validity")
			}
		}
		if err == nil {
			matched := re.Match(str)
			if re2matched != matched {
				fmt.Printf("re=%q str=%q regexp=%v re2=%v\n", restr, str, matched, re2matched)
				panic("regexp and re2 disagree on regexp match")
			}
		}
	}
	*/

	score := 0
	for _, ctor := range []func(string) (*regexp.Regexp, error){
		regexp.Compile,
		regexp.CompilePOSIX,
		func(str string) (*regexp.Regexp, error) {
			re, err := regexp.Compile(str)
			if err != nil {
				return re, err
			}
			re.Longest()
			return re, nil
		},
		func(str string) (*regexp.Regexp, error) {
			re, err := regexp.CompilePOSIX(str)
			if err != nil {
				return re, err
			}
			re.Longest()
			return re, nil
		},
	} {
		re, err := ctor(restr)
		if err != nil {
			continue
		}
		score = 1

		prefix, complete := re.LiteralPrefix()
		if complete {
			// https://github.com/golang/go/issues/11175
			if false && !re.MatchString(prefix) {
				panic("complete prefix is not matched")
			}
		} else {
			// https://github.com/golang/go/issues/11172
			if false && re.MatchString(prefix) {
				panic("partial prefix is matched")
			}
		}

		re.SubexpNames()
		re.NumSubexp()

		re.Split(sstr, 1)
		re.Split(sstr, -1)

		re.FindAll(str, 1)
		re.FindAll(str, -1)
		re.FindAllSubmatch(str, 1)
		re.FindAllSubmatch(str, -1)

		str1 := str[:len(str)/2]
		str2 := str[len(str)/2:]
		match := re.FindSubmatchIndex(str1)
		re.Expand(nil, str2, str1, match)

		re.ReplaceAll(str1, str2)
		re.ReplaceAllLiteral(str1, str2)
	}
	return score
}
