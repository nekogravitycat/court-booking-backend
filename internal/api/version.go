package api

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// VersionInfo reports the git revision this binary was built from, read from
// the VCS metadata the Go toolchain auto-embeds at build time (requires .git
// to be present in the build context, see .dockerignore). Lets a deployer
// confirm GET /v1/version in Swagger matches the commit they just pushed.
func VersionInfo(c *gin.Context) {
	commit, buildTime, dirty := "unknown", "unknown", false

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.time":
				buildTime = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"git_commit": commit,
		"build_time": buildTime,
		"dirty":      dirty,
	})
}
