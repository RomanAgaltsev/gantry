package forge

import "strings"

// IsPrerelease reports whether a semver string carries a prerelease segment (SemVer §9: the
// part after the first "-", before any "+" build metadata). An empty string is treated as a
// stable release so gantry never skips a release that simply lacks a semver_version. Used to
// keep GitLab's "latest release" (which includes RCs) aligned with GitHub's /releases/latest.
func IsPrerelease(semver string) bool {
	v := strings.TrimPrefix(strings.TrimSpace(semver), "v")
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus] // drop build metadata before checking for a prerelease
	}
	return strings.IndexByte(v, '-') >= 0
}
