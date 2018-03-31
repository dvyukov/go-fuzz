// Prevent go install ./... from complaining about different packages in the same dir.
// +build

package regexp

import (
	"regexp"
)

// START OMIT
func FuzzRegexp(data []byte) int {
	if len(data) < 3 {
		return 0
	}
	longestMode := data[0]%2 != 0  // first byte as "longest" flag
	reStr := data[1 : len(data)/2] // half as regular expression
	matchStr := data[len(data)/2:] // the rest is string to match

	re, err := regexp.Compile(string(reStr))
	if err != nil {
		return 0
	}
	if longestMode {
		re.Longest()
	}
	re.FindAll(matchStr, -1)
	return 0
}

// END OMIT
