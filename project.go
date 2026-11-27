package main

import (
	"path/filepath"
	"slices"

	caution "github.com/nullentropy/caution"
)

func projectOf(m Meta) string {
	if m.Repo != "" {
		return filepath.Base(m.Repo)
	}
	return filepath.Base(m.Cwd)
}

func (v *View) projects() []string {
	var names []string
	for _, a := range v.order {
		if t := v.tiles[a]; t != nil {
			if p := projectOf(t.snap.Meta); !slices.Contains(names, p) {
				names = append(names, p)
			}
		}
	}
	slices.Sort(names)
	return names
}

func (v *View) syncProjects() {
	names := v.projects()
	if v.project != "" && !slices.Contains(names, v.project) {
		v.project = ""
		v.layoutBoard()
	}
	if slices.Equal(names, v.projNames) && v.projSel != nil {
		v.projSel.SetSelected(v.projIndex())
		return
	}
	v.projNames = names
	v.projBox.Clear()
	if len(names) < 2 {
		v.projSel = nil
		return
	}
	opts := append([]string{"All projects"}, names...)
	v.projSel = caution.Select(opts, v.projIndex()).Tip("Show one project's sessions").OnSelect(func(i int) {
		v.project = ""
		if i > 0 {
			v.project = names[i-1]
		}
		v.layoutBoard()
	})
	v.projBox.Add(v.projSel.W(170))
}

func (v *View) filterProject(name string) {
	if v.project == name {
		name = ""
	}
	v.project = name
	v.layoutBoard()
	if v.projSel != nil {
		v.projSel.SetSelected(v.projIndex())
	}
}

func (v *View) projIndex() int {
	if i := slices.Index(v.projNames, v.project); i >= 0 && v.project != "" {
		return i + 1
	}
	return 0
}
