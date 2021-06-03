module github.com/dvyukov/go-fuzz/test

go 1.16

replace non.existent.com/foo => ./vendor/non.existent.com/foo/

require non.existent.com/foo v0.0.0-00010101000000-000000000000
