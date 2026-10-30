package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// A project is chats that share what their model is told, as Claude's
// and ChatGPT's are: its name, and its instructions, which go after the
// user's own with every message of its chats (withInstructions). The
// sidebar lists the projects first, in a section of their own, which
// folds, a button on its title's right making one; a project shows on a
// page of its own, which a chat's selection names "project:" and its ID,
// so that Back and Forward go to it as to a chat: its name over the
// composer, which starts a chat in it, and under it its instructions and
// its chats. Its chats show under its row in the sidebar, which unfolds
// them, the latest first, and not in Chats; pinned, in Pinned alone, its
// name by their model. Its name is in their title bar, which goes back
// to its page. projects.json keeps them; a chat keeps its
// project's ID. Removing a project keeps its chats, out of any.

// project is a project: its ID, as its chats keep it, its name and its
// instructions.
type project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Instructions string    `json:"instructions,omitempty"`
	Created      time.Time `json:"created"`
}

// projectPrefix starts the selection of a project's page.
const projectPrefix = "project:"

// project returns the project of id, nil for none.
func (a *app) project(id string) *project {
	for _, p := range a.projects {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// projectOf returns th's project, nil for none, or one removed.
func (a *app) projectOf(th *thread) *project {
	if th == nil || th.Project == "" {
		return nil
	}
	return a.project(th.Project)
}

// shownProject returns the project whose page shows, nil for none.
func (a *app) shownProject() *project {
	if id, ok := strings.CutPrefix(a.selected, projectPrefix); ok {
		return a.project(id)
	}
	return nil
}

// loadProjects reads the projects kept in dir, if any.
func (a *app) loadProjects(dir string) {
	a.projectsPath = filepath.Join(dir, "projects.json")
	b, err := os.ReadFile(a.projectsPath)
	if err != nil {
		return
	}
	json.Unmarshal(b, &a.projects)
}

// saveProjects keeps the projects, where the app has a place for them (not
// in tests), on a goroutine, as save does the chats.
func (a *app) saveProjects() {
	if a.projectsPath == "" {
		return
	}
	b, err := json.MarshalIndent(a.projects, "", "  ")
	if err != nil {
		return
	}
	a.projectsSaver.save(a.projectsPath, b)
}

// newProject makes a project, named New project (and a number where one
// is), shows its page, and its name in the title bar as a field, to type
// its own.
func (a *app) newProject() {
	name := locales.NewProject()
	for n := 2; slices.ContainsFunc(a.projects, func(p *project) bool { return p.Name == name }); n++ {
		name = locales.NewProject() + " " + strconv.Itoa(n)
	}
	var id [6]byte
	rand.Read(id[:])
	p := &project{ID: hex.EncodeToString(id[:]), Name: name, Created: time.Now()}
	a.projects = append(a.projects, p)
	a.foldProjects = false
	a.rev++
	a.saveProjects()
	a.open(projectPrefix + p.ID)
	a.startRenameProject(p)
}

// startRenameProject turns p's name, in the title bar of its page, into a
// field that renames it, as a chat's title is (startRename): after the
// page is shown.
func (a *app) startRenameProject(p *project) {
	a.commitRename()
	a.renaming, a.renameDraft, a.renameInSidebar = projectPrefix+p.ID, p.Name, false
	a.renameSelect, a.renameFocused, a.renameBuilt = true, false, true
	// The name takes the focus, not the composer, which showing the page
	// gave it (open), and whose taking it would end the renaming.
	a.focusDraft = false
}

// moveToProject moves th to the project id, "" for none.
func (a *app) moveToProject(th *thread, id string) {
	if th.Project == id {
		return
	}
	th.Project = id
	// Shown, it shows under the project, unfolded.
	if strconv.Itoa(th.ID) == a.selected {
		a.unfoldProject(id)
	}
	a.rev++
	a.save()
}

// askRemoveProject asks whether to remove p, which the alert does once it
// is confirmed (removeProject).
func (a *app) askRemoveProject(p *project) {
	a.removingProject, a.confirming = p.ID, true
}

// removeProject removes the project id and its instructions, its chats
// kept, out of any project; its page, where it shows, goes to a new chat.
func (a *app) removeProject(id string) {
	for _, th := range a.threads {
		if th.Project == id {
			th.Project = ""
		}
	}
	a.projects = slices.DeleteFunc(a.projects, func(p *project) bool { return p.ID == id })
	delete(a.projectDrafts, id)
	delete(a.projectFiles, id)
	delete(a.openProjects, id)
	page := projectPrefix + id
	if a.selected == page {
		a.selected, a.draft, a.files = "", a.newDraft, a.newFiles
	}
	a.nav.remove(page, a.selected)
	a.rev++
	a.save()
	a.saveProjects()
}

// projectMenu fills m, p's menu, in the title bar of its page or, in its
// row's of the sidebar: Rename…, which shows its page to rename it there,
// and Remove project….
func (a *app) projectMenu(m *ui.Menu, p *project) {
	if m.Item(locales.RenameChatItem()).Chosen() {
		a.open(projectPrefix + p.ID)
		a.startRenameProject(p)
	}
	m.Separator()
	if m.Item(locales.RemoveProjectItem()).Chosen() {
		a.askRemoveProject(p)
	}
}

// moveMenu fills m, a chat's menu, with Move to project, the projects
// under it, th's checked, and Remove from project where it is in one.
func (a *app) moveMenu(m *ui.Menu, th *thread) {
	if len(a.projects) == 0 {
		return
	}
	m.Submenu(locales.MoveToProject(), func(sub *ui.Menu) {
		for _, p := range a.projects {
			if sub.Item(p.Name).Checked(th.Project == p.ID).Chosen() {
				a.moveToProject(th, p.ID)
			}
		}
		if a.projectOf(th) != nil {
			sub.Separator()
			if sub.Item(locales.RemoveFromProject()).Chosen() {
				a.moveToProject(th, "")
			}
		}
	})
}

// projectRow builds p's row of the sidebar: its folder, and its name, at
// 500 on the 7% wash while its page shows, as a chat shown is; its menu
// on a right click; a click shows its page. Under the pointer, a chevron
// takes the folder's place, which unfolds its chats under it, or folds
// them, as VS Code's and Notion's do: beside the row rather than in it,
// the pointer over either keeping the row's wash.
func (a *app) projectRow(c *ui.Context, p *project) {
	t := c.Theme()
	page := projectPrefix + p.ID
	onPage, open := a.selected == page, a.openProjects[p.ID]
	box := ui.Box(c.Key("project " + p.ID))
	hover := box.Hovered()
	box.Children(func() {
		row := ui.ButtonBase(c.Key("row")).Margin(rowGap/2, barClear, rowGap/2, 8).Padding(7, 12).Gap(10).Radius(8).
			Justify(ui.Start).Label(p.Name)
		switch {
		case onPage:
			row.Background(a.pal.fg.Alpha(0.07))
		case hover:
			row.Background(a.pal.fg.Alpha(0.04))
		}
		row.Children(func() {
			ui.Icon(c, folderIcon).Size(16, 16).Shrink(0).TextColor(t.TextMuted).Opacity(shown(!hover))
			name := ui.Text(c, p.Name).SingleLine().MinWidth(0)
			if onPage {
				name.FontWeight(500)
			}
		})
		row.ContextMenu(func(m *ui.Menu) { a.projectMenu(m, p) })
		if row.Clicked() {
			a.open(page)
		}
		// Over the folder, its button as large as an icon button's wash
		// round it.
		const size = 22
		chev := ui.ButtonBase(c.Key("unfold")).Size(size, size).Radius(6).Absolute().Left(8 + 12 + 8 - size/2).
			Top(rowGap/2 + 7 + 8 - size/2).TextColor(t.TextMuted).Label(locales.ProjectChatsToggle(p.Name)).Expanded(open)
		chev.Opacity(shown(hover))
		if chev.Hovered() {
			chev.Background(a.pal.fg.Alpha(0.06)).TextColor(t.Text)
		}
		chev.Children(func() {
			icon := ui.Icon(c, chevronIcon).Size(14, 14)
			if !open {
				icon.Rotate(-90)
			}
		})
		// Once the list is built, whose rows it changes.
		chev.OnClick(func() { a.foldProject(p.ID, open) })
	})
}

// projectIndent is how much further in a chat's row under its project's
// is than a row of the sidebar, for its title to be where the project's
// name is: past its folder and the gap.
const projectIndent = 16 + 10

// projectChatsShown is how many of a project's chats show under it,
// unfolded, at most, the latest updated: View all shows its page, which
// lists them all.
const projectChatsShown = 5

// projectRows adds to rows p's chats, under its row unfolded: those out of
// Pinned, where they show, the latest updated first, projectChatsShown at
// most, but the one shown too, wherever it is, and View all, where there
// are more.
func (a *app) projectRows(rows []chatRow, p *project) []chatRow {
	var chats []*thread
	for _, th := range a.threads {
		if th.Project == p.ID && th.Pinned.IsZero() {
			chats = append(chats, th)
		}
	}
	slices.SortStableFunc(chats, func(x, y *thread) int { return y.Updated.Compare(x.Updated) })
	shown := chats[:min(len(chats), projectChatsShown)]
	if i := slices.IndexFunc(chats, func(th *thread) bool { return strconv.Itoa(th.ID) == a.selected }); i >= len(shown) {
		shown = append(slices.Clone(shown), chats[i])
	}
	for _, th := range shown {
		rows = append(rows, chatRow{th: th, nested: true})
	}
	if len(chats) > len(shown) {
		rows = append(rows, chatRow{viewAll: p})
	}
	return rows
}

// viewAllRow builds the row under p's chats unfolded, where it has more
// than show, which shows its page: muted, as far in as they are, the
// pointer bringing it up the ladder.
func (a *app) viewAllRow(c *ui.Context, p *project) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("view all "+p.ID)).Margin(rowGap/2, barClear, rowGap/2, 8+projectIndent).Padding(6, 12).
		Radius(8).Justify(ui.Start).TextColor(t.TextMuted).Label(locales.ViewAll())
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.04)).TextColor(t.Text)
	}
	b.Children(func() { ui.Text(c, locales.ViewAll()).FontSize(12).SingleLine() })
	if b.Clicked() {
		a.open(projectPrefix + p.ID)
	}
}

// foldProject folds the project id's chats in the sidebar, where open,
// else unfolds them; layout.json keeps which are unfolded.
func (a *app) foldProject(id string, open bool) {
	if a.openProjects == nil {
		a.openProjects = map[string]bool{}
	}
	if open {
		delete(a.openProjects, id)
	} else {
		a.openProjects[id] = true
	}
	a.rev++
	a.saveLayout()
}

// unfoldProject unfolds the project id's chats in the sidebar, where they
// are folded: as one of them shows, for its row to.
func (a *app) unfoldProject(id string) {
	if id != "" && !a.openProjects[id] {
		a.foldProject(id, false)
	}
}

// projectHeader builds the top of p's page, over the composer, as an empty
// chat's greeting is: its folder, muted, over its name at 26.
func (a *app) projectHeader(c *ui.Context, p *project) {
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.End).AlignItems(ui.Center).Padding(24, 24, 28, 24).Gap(12).
		Children(func() {
			ui.Icon(c, folderIcon).Size(40, 40).TextColor(c.Theme().TextMuted)
			ui.Text(c, p.Name).FontSize(26).SingleLine().MaxWidth(maxWidth)
		})
}

// projectBody builds the bottom of p's page, under the composer, as wide
// as it, scrolling where it is taller than its room: p's instructions, a
// field on a card, kept as typed, and its chats, the latest updated
// first, each a row that shows it.
func (a *app) projectBody(c *ui.Context, p *project) {
	t := c.Theme()
	label := func(s string) { ui.Text(c, s).FontSize(12).FontWeight(500).TextColor(t.TextMuted).Padding(0, 4) }
	ui.Column(c).Grow(1.2).MinHeight(0).AlignItems(ui.Center).Padding(4, 24, 16, 24).Children(func() {
		ui.Scroll(c).WidthPercent(100).MaxWidth(maxWidth).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(8).AlignItems(ui.Stretch).Padding(8, 0, 0, 0).Children(func() {
				label(locales.ProjectInstructions())
				ui.Box(c).Radius(12).Border(1, t.Border).Background(a.pal.composer).Children(func() {
					field := ui.TextAreaBase(c.Key("project instructions "+p.ID), &p.Instructions).WidthPercent(100).Lines(3, 10).
						FontSize(14).LineHeight(1.5).Padding(10, 12).Label(locales.ProjectInstructions())
					a.placeholder(c, locales.ProjectInstructionsPlaceholder(), p.Instructions == "" && !field.Composing(), 10, 12, 14, 1.5)
					if field.Changed() {
						a.saveProjects()
					}
				})
				ui.Text(c, locales.ProjectInstructionsAbout()).FontSize(12).LineHeight(1.5).TextColor(t.TextMuted).Padding(0, 4)

				ui.Box(c).Height(16)
				label(locales.SidebarChats())
				var chats []*thread
				for _, th := range a.threads {
					if th.Project == p.ID {
						chats = append(chats, th)
					}
				}
				if len(chats) == 0 {
					ui.Text(c, locales.ProjectNoChats()).FontSize(13).TextColor(t.TextMuted).Padding(6, 4)
					return
				}
				slices.SortStableFunc(chats, func(x, y *thread) int { return y.Updated.Compare(x.Updated) })
				now := time.Now()
				for _, th := range chats {
					row := ui.ButtonBase(c.Key("project chat "+strconv.Itoa(th.ID))).Padding(10, 12).Gap(12).Radius(8).
						Justify(ui.Start).AlignItems(ui.Center).Label(th.Title)
					if row.Hovered() {
						row.Background(a.pal.fg.Alpha(0.04))
					}
					row.Children(func() {
						ui.Icon(c, messageIcon).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
						ui.Text(c, th.Title).FontSize(14).SingleLine().MinWidth(0).Grow(1)
						ui.Text(c, ago(th.Updated, now)).FontSize(12).TextColor(t.TextMuted).Shrink(0)
					})
					if row.Clicked() {
						a.open(strconv.Itoa(th.ID))
					}
				}
			})
		})
	})
}

// projectCrumb builds, in the title bar of p's chat, before its title,
// p's name, muted, which shows its page, as a path does, over a slash.
func (a *app) projectCrumb(c *ui.Context, p *project) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("crumb")).Padding(4, 6).Margin(0, 0, 0, -6).Radius(6).TextColor(t.TextMuted).Label(p.Name).Shrink(1).MinWidth(0)
	if b.Hovered() {
		b.Background(a.pal.fg.Alpha(0.06)).TextColor(t.Text)
	}
	b.Children(func() { ui.Text(c, p.Name).FontSize(14).SingleLine().MaxWidth(200) })
	if b.Clicked() {
		a.open(projectPrefix + p.ID)
	}
	ui.Text(c, "/").FontSize(14).TextColor(t.TextMuted).Padding(0, 6, 0, 2).Shrink(0)
}
