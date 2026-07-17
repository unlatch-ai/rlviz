// Package fixtures embeds synthetic public data used by product demos.
package fixtures

import _ "embed"

// DemoNDJSON is the rich synthetic rollout group opened by `rlviz demo`.
//
//go:embed canonical/demo.ndjson
var DemoNDJSON []byte
