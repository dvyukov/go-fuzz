// Copyright 2019 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package fuzz

// F is a type passed to Fuzz functions to manage fuzzing state.
type F interface {
	// Fatal is equivalent to Log followed by FailNow.
	Fatal(args ...interface{})

	// Fatalf is equivalent to Logf followed by FailNow.
	Fatalf(format string, args ...interface{})

	// Skip is equivalent to Log followed by SkipNow.
	Skip(args ...interface{})

	// Skipf is equivalent to Logf followed by SkipNow.
	Skipf(format string, args ...interface{})

	// Log formats its arguments using default formatting,
	// analogous to Println, and records the text in an error log.
	// A final newline is added if not provided.
	// The error log is discarded at the end of every fuzz function invocation;
	// output is recorded only when the fuzz function fails.
	// Logging slows down fuzzing and should be used sparingly.
	Log(args ...interface{})

	// Logf formats its arguments according to the format,
	// analogous to Printf, and records the text in an error log.
	// A final newline is added if not provided.
	// The error log is discarded at the end of every fuzz function invocation;
	// output is recorded only when the fuzz function fails.
	// Logging slows down fuzzing and should be used sparingly.
	Logf(format string, args ...interface{})

	// Interesting tells go-fuzz that this input is interesting and should be given added priority.
	// For example, if fuzzing a JSON decoder, syntactically correct JSON inputs might be marked as interesting.
	// Interesting may be called multiple times; each call will increase priority.
	Interesting()

	// Name reports the name of the fuzz function.
	Name() string

	// Error is equivalent to Fatal.
	// F has both in order to match the testing.TB interface.
	Error(args ...interface{})

	// Errorf is equivalent to Fatalf.
	// F has both in order to match the testing.TB interface.
	Errorf(format string, args ...interface{})

	// FailNow marks the function as having failed and stops execution of the fuzz function.
	FailNow()

	// SkipNow tells go-fuzz that this input should not added to the corpus and stops execution of the fuzz function.
	SkipNow()

	// Fail is equivalent to FailNow.
	// F has both in order to match the testing.TB interface.
	Fail()

	// Failed is unused. It has no effect and always returns false.
	// It is present only in order to match the testing.TB interface.
	Failed() bool

	// Skipped is unused. It has no effect and always returns false.
	// It is present only in order to match the testing.TB interface.
	Skipped() bool

	// Helper is unused. Calling it has no effect.
	// It is present only in order to match the testing.TB interface.
	Helper()
}
