package daemon

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
)

// Build identifies the running binary, computed once, the first time it is
// asked for.
//
// It is the VCS revision and modified flag go build stamps, plus the size and
// modification time of the executable. The second half is what changes when a
// dirty tree is rebuilt, where the VCS fields stay the same. Once matters: a
// running binary that stats os.Executable after go install replaced the file
// sees the new file, so a daemon computing this per request would always match
// the client it was meant to warn.
var Build = sync.OnceValue(computeBuild)

// computeBuild reads the identifier's parts.
func computeBuild() string {
	revision, modified := "unknown", ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					modified = "+dirty"
				}
			}
		}
	}
	exe := "unknown"
	if path, err := os.Executable(); err == nil {
		if fi, err := os.Stat(path); err == nil {
			exe = fmt.Sprintf("%d@%d", fi.Size(), fi.ModTime().Unix())
		}
	}
	return revision + modified + " " + exe
}
