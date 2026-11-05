package server

import "fmt"

type reportChangeGroup struct {
	ID, Section             string
	Added, Removed, Changed int
	Sets                    []reportChangeSet
}

type reportChangeSet struct {
	Label   string
	Changes []documentChange
}

func groupReportChanges(changes []documentChange) []reportChangeGroup {
	var groups []reportChangeGroup
	indices := map[string]int{}
	for _, change := range changes {
		index, ok := indices[change.Section]
		if !ok {
			index = len(groups)
			indices[change.Section] = index
			groups = append(groups, reportChangeGroup{ID: fmt.Sprintf("report-section-%d", index+1), Section: change.Section, Sets: []reportChangeSet{{Label: "Changed"}, {Label: "Removed parts"}, {Label: "Added parts"}}})
		}
		group := &groups[index]
		set := 0
		switch change.Kind {
		case "Added":
			group.Added++
			set = 2
		case "Removed":
			group.Removed++
			set = 1
		default:
			group.Changed++
		}
		group.Sets[set].Changes = append(group.Sets[set].Changes, change)
	}
	return groups
}
