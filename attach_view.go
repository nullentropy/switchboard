package main

import (
	"fmt"
	"slices"
	"time"

	caution "github.com/nullentropy/caution"
)

type tray struct {
	v     *View
	imgs  []Image
	box   *caution.Node
	chips *caution.Node
	note  *caution.Node
	said  int
}

func newTray(v *View, bg string) *tray {
	t := &tray{v: v}
	t.chips = caution.HStack().Gap(8).Align("center")
	t.note = caution.Label("").FontSize(12).Color("$stNeed")
	t.box = caution.Panel().Dock("bottom").H(0).Bg(bg).Clips().Kids(
		caution.VStack().Gap(6).Anchor(caution.A{Left: px(20), Right: px(20), CenterY: px(0)}).Kids(t.note, t.chips))
	return t
}

func (t *tray) height() float64 {
	h := 0.0
	if len(t.imgs) > 0 {
		h += thumbH + 16
	}
	if valueText(t.note) != "" {
		h += 26
	}
	return h
}

func valueText(n *caution.Node) string {
	s, _ := n.Prop("text").(string)
	return s
}

func (t *tray) say(msg string) {
	t.said++
	n := t.said
	t.note.SetText(msg)
	t.box.H(t.height())
	if msg == "" {
		return
	}
	time.AfterFunc(6*time.Second, func() {
		t.v.s.Update(func() {
			if t.said == n {
				t.say("")
			}
		})
	})
}

func (t *tray) add(files []caution.File) {
	go func() {
		var got []Image
		var errs []string
		for _, f := range files {
			im, err := newImage(f.Name, f.Data)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			got = append(got, im)
		}
		t.v.s.Update(func() {
			for _, im := range got {
				if slices.ContainsFunc(t.imgs, func(x Image) bool { return x.ID == im.ID }) {
					continue
				}
				if len(t.imgs) >= maxImages {
					errs = append(errs, fmt.Sprintf("A message can carry up to %d images.", maxImages))
					break
				}
				t.imgs = append(t.imgs, im)
			}
			t.draw()
			if len(errs) > 0 {
				t.say(errs[0])
			} else {
				t.say("")
			}
		})
	}()
}

func (t *tray) remove(id string) {
	t.imgs = slices.DeleteFunc(t.imgs, func(im Image) bool { return im.ID == id })
	t.draw()
}

func (t *tray) take() []Image {
	imgs := t.imgs
	t.imgs = nil
	t.draw()
	return imgs
}

func (t *tray) draw() {
	t.chips.Clear()
	for _, im := range t.imgs {
		id := im.ID
		t.chips.Add(caution.Panel().W(max(im.W, 24)).H(thumbH).Radius(6).Border("$edgeSoft", 1).Clips().Kids(
			caution.Image(im.Thumb).W(im.W).H(im.H).Anchor(caution.A{CenterX: px(0), CenterY: px(0)}),
			caution.Panel().W(18).H(18).Radius(9).Bg("$titlebar").Border("$edgeSoft", 1).Anchor(caution.A{Right: px(3), Top: px(3)}).Kids(
				caution.Label("×").FontSize(13).Color("$ink").Anchor(caution.A{CenterX: px(0), CenterY: px(0)}),
				caution.Glass().HoverFill("$hover").Anchor(fill()).Tip("Remove this image").
					OnPick(func(_, _ float64, _ *caution.Node) { t.remove(id) }),
			),
		))
	}
	t.box.H(t.height())
}

func thumbs(imgs []Image) *caution.Node {
	row := caution.HStack().Gap(8)
	for _, im := range imgs {
		row.Add(caution.Panel().W(im.W).H(im.H).Radius(6).Clips().Kids(
			caution.Image(im.Thumb).W(im.W).H(im.H).Anchor(fill())))
	}
	return row
}

func (v *View) dropFiles(files []caution.File, x, y float64) {
	if v.focus != nil {
		t := v.focus.tray
		switch {
		case v.dialog != nil:
			t.say("Images can only be attached to a message box.")
		case x < sidebarWidth(float64(v.set.SidebarW)):
			t.say("Drop images on the conversation or its message box.")
		default:
			t.add(files)
		}
		return
	}
	_, h := v.s.Viewport()
	if v.dialog == nil && y >= h-consoleH-v.cmdTray.height() {
		v.cmdTray.add(files)
		return
	}
	v.cmdTray.say("Drop images on the dispatcher bar below, or open a session and drop them there.")
}
