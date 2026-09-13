package style

import _ "embed"

// Conventions is the document the rules in this package enforce. It is
// embedded rather than read from docs/ so a single binary carries it: the
// rules and the text explaining them have to ship together, or the error
// message points at a file the caller cannot open.
//
//go:embed conventions.md
var Conventions string
