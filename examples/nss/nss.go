// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package nss

/*
import (
	"bytes"
	"net"
)
*/

func Fuzz(data []byte) int {
	// This example won't build as is, because ParseNSSConf function is not exported
	// from net package. To build this example, you need to patch net package to
	// rename parseNSSConf to ParseNSSConf first.
	/*
		if net.ParseNSSConf(bytes.NewReader(data)) == nil {
			return 0
		}
	*/
	return 1
}
