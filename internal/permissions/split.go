// SPDX-License-Identifier: Apache-2.0

package permissions

import "slices"

// splitInSpec says in which spec each permission of SplitFrom was split off its broader
// one. A list of permissions written before that spec knew none of that spec's finer
// permissions.
var splitInSpec = map[string]int{
	SitePublish: 36, SpaceAutomation: 36, SpaceTemplates: 36, SpaceAudit: 36,
	SpaceLabels: 40, SpaceStatuses: 40, SpaceApprovals: 40, SpaceBundle: 40, SpaceLook: 40,
	PageWatchers: 40, PageMoveOut: 40, SpaceInviteGuests: 40,
}

// WithSplitOff completes a list of single permissions that may have been written before a
// split, such as by a script on the older grants route. Where the list holds a broader
// permission and not one of the permissions a spec split off it, it was written without
// them, and it gets all of them, as the migration of that spec gave them. A list that
// holds one of them already says what it means, and stays as it is.
func WithSplitOff(perms []string) []string {
	out := slices.Clone(perms)
	type group struct {
		broad string
		spec  int
	}
	finer := map[group][]string{}
	for f, broad := range SplitFrom {
		g := group{broad, splitInSpec[f]}
		finer[g] = append(finer[g], f)
	}
	for g, list := range finer {
		if !slices.Contains(perms, g.broad) || slices.ContainsFunc(list, func(f string) bool { return slices.Contains(perms, f) }) {
			continue
		}
		for _, f := range list {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	slices.Sort(out)
	return out
}
