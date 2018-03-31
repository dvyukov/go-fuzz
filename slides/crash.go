// Prevent go install ./... from complaining about different packages in the same dir.
// +build

package n

type R interface{ S }
type S = interface{ R }
