package main

import (
	"fmt"
	"golang.org/x/net/spdy"
	"net/http"
	"os"
)

func main() {
	headers := make(http.Header)
	headers["foo"] = []string{}
	headers["bar"] = []string{"foo"}
	headers["baz"] = []string{"foo", "bar"}
	write(&spdy.DataFrame{11, 0, []byte{}})
	write(&spdy.DataFrame{11, 0, []byte("foo")})
	write(&spdy.DataFrame{11, spdy.DataFlagFin, []byte{}})
	write(&spdy.DataFrame{11, spdy.DataFlagFin, []byte("write(spdy.DataFrame{11, DataFlagFin, []byte(foo)})")})
	write(&spdy.GoAwayFrame{CFHeader: spdy.ControlFrameHeader{}, LastGoodStreamId: 42, Status: spdy.GoAwayOK})
	write(&spdy.GoAwayFrame{CFHeader: spdy.ControlFrameHeader{}, LastGoodStreamId: 42 | (1 << 31), Status: spdy.GoAwayOK})
	write(&spdy.GoAwayFrame{CFHeader: spdy.ControlFrameHeader{Flags: spdy.ControlFlagFin}, LastGoodStreamId: 42, Status: spdy.GoAwayProtocolError})
	write(&spdy.GoAwayFrame{CFHeader: spdy.ControlFrameHeader{Flags: spdy.ControlFlagFin | spdy.ControlFlagUnidirectional | spdy.ControlFlagSettingsClearSettings}, LastGoodStreamId: 42, Status: spdy.GoAwayInternalError})
	write(&spdy.HeadersFrame{CFHeader: spdy.ControlFrameHeader{}, StreamId: 42})
	write(&spdy.HeadersFrame{CFHeader: spdy.ControlFrameHeader{}, StreamId: 42 | (1 << 31), Headers: headers})
	write(&spdy.HeadersFrame{CFHeader: spdy.ControlFrameHeader{Flags: spdy.ControlFlagFin}, StreamId: 42, Headers: headers})
	write(&spdy.HeadersFrame{CFHeader: spdy.ControlFrameHeader{Flags: spdy.ControlFlagFin | spdy.ControlFlagUnidirectional | spdy.ControlFlagSettingsClearSettings}, StreamId: 42, Headers: headers})
	write(&spdy.PingFrame{Id: 11})
	write(&spdy.RstStreamFrame{CFHeader: spdy.ControlFrameHeader{Flags: spdy.ControlFlagFin | spdy.ControlFlagUnidirectional | spdy.ControlFlagSettingsClearSettings}, StreamId: 123123122, Status: spdy.InternalError})
	write(&spdy.SettingsFrame{FlagIdValues: []spdy.SettingsFlagIdValue{spdy.SettingsFlagIdValue{spdy.FlagSettingsPersistValue, spdy.SettingsUploadBandwidth, 0}}})
	write(&spdy.SettingsFrame{FlagIdValues: []spdy.SettingsFlagIdValue{spdy.SettingsFlagIdValue{spdy.FlagSettingsPersisted, spdy.SettingsDownloadBandwidth, 11}, spdy.SettingsFlagIdValue{spdy.FlagSettingsPersisted, spdy.SettingsCurrentCwnd, 123123123}}})
	write(&spdy.SettingsFrame{FlagIdValues: []spdy.SettingsFlagIdValue{spdy.SettingsFlagIdValue{spdy.FlagSettingsPersisted, spdy.SettingsDownloadRetransRate, 11}, spdy.SettingsFlagIdValue{spdy.FlagSettingsPersisted, spdy.SettingsClientCretificateVectorSize, 123123123}}})
	write(&spdy.SynReplyFrame{CFHeader: spdy.ControlFrameHeader{Flags: spdy.ControlFlagFin}, StreamId: 123123, Headers: headers})
	write(&spdy.SynStreamFrame{StreamId: 11, AssociatedToStreamId: 22, Priority: 5, Slot: 17, Headers: headers})
	write(&spdy.WindowUpdateFrame{StreamId: 11, DeltaWindowSize: 123123123})
}

func write(frame spdy.Frame) {
	f, err := os.Create(fmt.Sprintf("/tmp/spdy/%d", seq))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	seq++
	framer, err := spdy.NewFramer(f, nil)
	if err != nil {
		panic(err)
	}
	err = framer.WriteFrame(frame)
	if err != nil {
		panic(err)
	}
}

var seq int
