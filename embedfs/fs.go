package embedfs

import "embed"

// /* embeds with all files, just dir/ ignores files starting with _ or .
//
//go:embed static templates
var Embedded embed.FS
