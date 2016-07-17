# writerset
--
    import "github.com/stephens2424/writerset"

Package writerset implements a mechanism to add and remove writers from a
construct similar to io.MultiWriter.

## Usage

#### type WriterSet

```go
type WriterSet struct {
}
```

WriterSet wraps multiple writers like io.MultiWriter, but such that individual
writers are easy to add or remove as necessary.

#### func  New

```go
func New() *WriterSet
```
New initializes a new empty writer set.

#### func (*WriterSet) Add

```go
func (ws *WriterSet) Add(w io.Writer) <-chan error
```
Add ensures w is in the set.

#### func (*WriterSet) Contains

```go
func (ws *WriterSet) Contains(w io.Writer) bool
```
Contains determines if w is in the set.

#### func (*WriterSet) Flush

```go
func (ws *WriterSet) Flush()
```
Flush implements http.Flusher by calling flush on all the underlying writers if
they are also http.Flushers.

#### func (*WriterSet) Remove

```go
func (ws *WriterSet) Remove(w io.Writer)
```
Remove ensures w is not in the set.

#### func (*WriterSet) Write

```go
func (ws *WriterSet) Write(b []byte) (int, error)
```
Write writes data to each underlying writer. If an error occurs on an underlying
writer, that writer is removed from the set. The error will be sent on the
channel created when the writer was added.
