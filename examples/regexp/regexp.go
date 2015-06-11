package regexp

import (
	"regexp"
)

func Fuzz(data []byte) int {
	str := data[:len(data)/2]
	sstr := string(str)
	restr := string(data[len(data)/2:])

	quoted := regexp.QuoteMeta(sstr)
	req, err := regexp.Compile(quoted)
	if err == nil {
		if !req.MatchString(sstr) {
			panic("quoted is not matched")
		}
	}

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
