// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

/*
#include <re2/re2.h>
#include <string.h>

// To build this you need to run go-fuzz-build as:
// CGO_CXXFLAGS="-I /path/to/re2" CGO_LDFLAGS="/path/to/re2/obj/libre2.a" go-fuzz-build github.com/dvyukov/go-fuzz/examples/regexp

extern "C"
int RE2Match(char* restr, int restrlen, char* str, int strlen, int* matched, char** error) {
	RE2 re(std::string(restr, restrlen), RE2::Quiet);
	if (!re.ok()) {
		*error = strdup(re.error().c_str());
		return 0;
	}
	*matched = RE2::PartialMatch(std::string(str, strlen), re);
	return 1;
}
*/
