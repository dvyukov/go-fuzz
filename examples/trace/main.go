// This file needs to be copied somewhere into GOROOT/src,
// otherwise it will fail to import internal packages.

package trace

import (
	"bytes"
	"internal/trace"
)

func Fuzz(data []byte) int {
	events, err := trace.Parse(bytes.NewReader(data))
	if err != nil {
		if events != nil {
			panic("events is not nil on error")
		}
		return 0
	}
	trace.GoroutineStats(events)
	trace.RelatedGoroutines(events, 1)
	return 1
}
