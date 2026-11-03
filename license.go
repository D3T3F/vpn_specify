// Package vpnspecify exposes files from the project root that are embedded in
// the executables (go:embed only reaches the package's own directory).
package vpnspecify

import _ "embed"

// License is the text of the LICENSE file (MIT).
//
//go:embed LICENSE
var License string
