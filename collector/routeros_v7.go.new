package collector

import (
	"fmt"
	"regexp"
	"strconv"
)

var (
	sessionSuffix   = regexp.MustCompile(`-\d+$`)
	filterRuleFinal = regexp.MustCompile(`\{\s*(\w+)\s*;\s*\}\s*$`)
	versionMajor    = regexp.MustCompile(`^(\d+)\.`)
)

// routerOSMajor selects v6/v7 menus up front: probing a missing menu returns !trap, and the sync
// client of routeros.v2 leaves the trailing !done unread, desynchronizing the next replies.
func routerOSMajor(ctx *collectorContext) (int, error) {
	reply, err := ctx.client.Run("/system/resource/print", "=.proplist=version")
	if err != nil {
		return 0, err
	}
	if len(reply.Re) == 0 {
		return 0, fmt.Errorf("empty /system/resource reply")
	}
	m := versionMajor.FindStringSubmatch(reply.Re[0].Map["version"])
	if m == nil {
		return 0, fmt.Errorf("unexpected RouterOS version %q", reply.Re[0].Map["version"])
	}
	return strconv.Atoi(m[1])
}

// sessionConnection maps a RouterOS v7 BGP session name (<connection>-<n>) to its connection name.
func sessionConnection(name string) string {
	return sessionSuffix.ReplaceAllString(name, "")
}

// filterRuleAction returns the action of a v7 rule ending in "{ <action>; }", or "" if unknown.
func filterRuleAction(rule string) string {
	m := filterRuleFinal.FindStringSubmatch(rule)
	if m == nil {
		return ""
	}
	return m[1]
}
