package strings

import (
	"strings"
	"unicode"
)

func Fuzz(data []byte) int {
	s := string(data)
	si := len(s) / 4 * 3
	s0 := s[:si]
	s1 := s[si:]
	s2, r := splitRune(s)
	s3 := s[:len(s)/2]
	s4 := s[len(s)/2:]
	s5 := s[:len(s)/3]
	s6 := s[len(s)/3 : len(s)/3*2]
	s7 := s[len(s)/3*2:]

	strings.Contains(s0, s1)
	strings.ContainsAny(s0, s1)
	strings.ContainsRune(s, r)
	strings.Count(s0, s1)
	strings.EqualFold(s3, s4)
	fields := strings.Fields(s)
	strings.HasPrefix(s0, s1)
	strings.HasSuffix(s0, s1)
	strings.Index(s0, s1)
	strings.IndexAny(s0, s1)
	strings.IndexByte(s2, byte(r))
	strings.IndexRune(s2, r)
	strings.Join(fields, " ")
	strings.LastIndex(s0, s1)
	strings.LastIndexAny(s0, s1)
	strings.Repeat(s, 2)
	strings.Replace(s, s0, s1, -1)
	strings.Split(s0, s1)
	strings.SplitAfter(s0, s1)
	strings.SplitAfterN(s0, s1, 2)
	strings.SplitN(s0, s1, 2)
	strings.Title(s)
	strings.ToLower(s)
	strings.ToLowerSpecial(unicode.AzeriCase, s)
	strings.ToTitle(s)
	strings.ToTitleSpecial(unicode.AzeriCase, s)
	strings.ToUpper(s)
	strings.ToUpperSpecial(unicode.AzeriCase, s)
	strings.Trim(s0, s1)
	strings.TrimLeft(s0, s1)
	strings.TrimPrefix(s0, s1)
	strings.TrimRight(s0, s1)
	strings.TrimSpace(s)
	strings.TrimSuffix(s0, s1)
	strings.NewReplacer(s5, s6).Replace(s7)
	return 0
}

func splitRune(s string) (string, rune) {
	for i, r := range s {
		return s[i:], r
	}
	return s, 0
}
