// Package gallerydata embeds the deterministic synthetic sources opened by rlviz demo.
package gallerydata

import "embed"

// Files contains the generated gallery sources.
//
//go:embed *.ndjson
var Files embed.FS

// Names is the stable registration order used by the demo command.
var Names = []string{"coding-agent-bugfix.ndjson", "web-research-agent.ndjson", "checkout-cohort.ndjson"}
