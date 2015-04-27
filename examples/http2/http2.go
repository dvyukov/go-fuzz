package http2

import (
	"bytes"
	"fmt"
	"github.com/bradfitz/http2"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	framer := http2.NewFramer(ioutil.Discard, bytes.NewReader(data))
	framer.SetMaxReadFrameSize(64 << 10)
	framer.AllowIllegalWrites = true
	for score := 0; ; score++ {
		f, err := framer.ReadFrame()
		if err != nil {
			if f != nil {
				panic(fmt.Sprintf("ReadFrame failed with '%v' but frame is not nil", err))
			}
			return score
		}
		switch ff := f.(type) {
		case *http2.ContinuationFrame:
			err = framer.WriteContinuation(ff.Header().StreamID, ff.HeadersEnded(), ff.HeaderBlockFragment())
		case *http2.DataFrame:
			err = framer.WriteData(ff.Header().StreamID, ff.StreamEnded(), ff.Data())
		case *http2.GoAwayFrame:
			err = framer.WriteGoAway(ff.LastStreamID, ff.ErrCode, ff.DebugData())
		case *http2.HeadersFrame:
			err = framer.WriteHeaders(http2.HeadersFrameParam{ff.Header().StreamID, ff.HeaderBlockFragment(), ff.StreamEnded(), ff.HeadersEnded(), 0, ff.Priority})
		case *http2.PingFrame:
			err = framer.WritePing(ff.Header().Flags&http2.FlagPingAck != 0, ff.Data)
		case *http2.PriorityFrame:
			err = framer.WritePriority(ff.Header().StreamID, ff.PriorityParam)
		case *http2.PushPromiseFrame:
			err = framer.WritePushPromise(http2.PushPromiseParam{ff.Header().StreamID, ff.PromiseID, ff.HeaderBlockFragment(), ff.HeadersEnded(), 0})
		case *http2.RSTStreamFrame:
			err = framer.WriteRSTStream(ff.Header().StreamID, ff.ErrCode)
		case *http2.SettingsFrame:
			var ss []http2.Setting
			ff.ForeachSetting(func(s http2.Setting) error {
				ss = append(ss, s)
				return nil
			})
			if ff.IsAck() {
				err = framer.WriteSettingsAck()
			} else {
				err = framer.WriteSettings(ss...)
			}
		case *http2.WindowUpdateFrame:
			err = framer.WriteWindowUpdate(ff.Header().StreamID, ff.Increment)
		case *http2.UnknownFrame:
			err = framer.WriteRawFrame(ff.Header().Type, ff.Header().Flags, ff.Header().StreamID, ff.Payload())
		default:
			panic(fmt.Sprintf("unknown frame type %+v", ff))
		}
		if err != nil {
			panic(fmt.Sprintf("WriteFrame failed with '%v'", err))
		}
	}
}
