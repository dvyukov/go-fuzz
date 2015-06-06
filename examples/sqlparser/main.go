package sqlparser

import (
	"github.com/youtube/vitess/go/vt/sqlparser"
)

func Fuzz(data []byte) int {
	stmt, err := sqlparser.Parse(string(data))
	if err != nil {
		if stmt != nil {
			panic("stmt is not nil on error")
		}
		return 0
	}
	buf := sqlparser.NewTrackedBuffer(nil)
	stmt.Format(buf)
	pq := buf.ParsedQuery()
	vars := map[string]interface{}{
		"A": 42,
		"B": 123123123,
		"C": "",
		"D": "a",
		"E": "foobar",
		"F": 1.1,
	}
	pq.GenerateQuery(vars)
	return 1
}
