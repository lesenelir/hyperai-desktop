package main

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hyperai-desktop/locales"
)

// projectApp is an app of a provider that keeps what its replies were
// asked of, in dir, its window tt.
func projectApp(t *testing.T, dir string) (*app, *ui.Tester, *[][]turn) {
	var asked [][]turn
	a := newApp(&recorder{provider: &counter{}, asked: &asked})
	a.loadFrom(dir)
	t.Cleanup(func() { a.saver.wait(); a.projectsSaver.wait(); a.layoutSaver.wait() })
	return a, ui.NewTester(a.view, 1120, 760), &asked
}

// New project, in the sidebar, makes one and shows its page, its name a
// field in the title bar, to type its own.
func TestNewProject(t *testing.T) {
	a, tt, _ := projectApp(t, t.TempDir())
	tt.Frame()
	if err := tt.Click(locales.NewProject()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(a.projects) != 1 || a.selected != projectPrefix+a.projects[0].ID || a.renaming != a.selected {
		t.Fatalf("projects %d, shown %q, renaming %q", len(a.projects), a.selected, a.renaming)
	}
	tt.Type("HyperAI")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if p := a.projects[0]; p.Name != "HyperAI" || a.renaming != "" {
		t.Fatalf("named %q, renaming %q", p.Name, a.renaming)
	}
	// Its name, in the sidebar (its row's name and its label), the title
	// bar, and over its composer.
	if n := slices.Index(tt.Texts(), "HyperAI"); n < 0 || len(slices.DeleteFunc(slices.Clone(tt.Texts()), func(s string) bool { return s != "HyperAI" })) < 4 {
		t.Errorf("the name shows in %q", tt.Texts())
	}
	if !tt.HasText(locales.ProjectNoChats()) || !tt.HasText(locales.ProjectInstructionsPlaceholder()) {
		t.Errorf("no page in %q", tt.Texts())
	}
	// Another is named apart.
	a.newProject()
	if a.projects[1].Name != locales.NewProject() {
		a.commitRename()
	}
	a.renaming = ""
	a.newProject()
	if got := a.projects[2].Name; got != locales.NewProject()+" 2" {
		t.Errorf("the third is named %q", got)
	}
}

// A chat started on a project's page is the project's: it is told the
// user's instructions, then the project's; it shows the project in the
// sidebar and its title bar, whose name goes back to the page, which
// lists it. The page keeps its own draft.
func TestProjectChat(t *testing.T) {
	a, tt, asked := projectApp(t, t.TempDir())
	a.instructions = "Reply in Chinese."
	a.newProject()
	a.renaming = ""
	p := a.projects[0]
	p.Name = "HyperAI"
	tt.Frame()
	if err := tt.Click(locales.ProjectInstructionsPlaceholder()); err != nil {
		t.Fatal(err)
	}
	tt.Type("It is a Go desktop app.")
	tt.Advance(time.Second)
	if p.Instructions != "It is a Go desktop app." {
		t.Fatalf("instructions %q", p.Instructions)
	}

	// The page's draft, its own.
	a.focusDraft = true
	tt.Frame()
	tt.Type("Half written")
	a.newChat()
	tt.Frame()
	if a.draft != "" {
		t.Errorf("a new chat holds the page's draft %q", a.draft)
	}
	a.goBack()
	tt.Frame()
	if a.draft != "Half written" {
		t.Errorf("the page's draft %q", a.draft)
	}
	a.draft = ""

	a.send("How do I lay out the sidebar?")
	tt.Frame()
	th := a.current()
	if th == nil || th.Project != p.ID {
		t.Fatalf("chat %+v", th)
	}
	if got := (*asked)[0]; got[0].Role != "system" || got[0].Content != "Reply in Chinese.\n\nIt is a Go desktop app." {
		t.Errorf("asked %+v", got)
	}
	// In the sidebar, under its project, unfolded as it started, and not in
	// Chats.
	if rows := a.chatRows(time.Now()); !slices.ContainsFunc(rows, func(r chatRow) bool { return r.th == th && r.nested }) ||
		slices.ContainsFunc(rows, func(r chatRow) bool { return r.th == th && !r.nested }) {
		t.Errorf("the sidebar's rows %+v", rows)
	}
	// Its title bar's crumb, back to the page, which lists it.
	if err := tt.Click("HyperAI"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != projectPrefix+p.ID || !tt.HasText("How do I lay out the sidebar?") || tt.HasText(locales.ProjectNoChats()) {
		t.Errorf("shown %q; texts %q", a.selected, tt.Texts())
	}
	// Back, to the chat.
	a.goBack()
	tt.Frame()
	if a.current() != th {
		t.Errorf("back to %q", a.selected)
	}
}

// A chat moves to a project from its menu, and out of it.
func TestMoveToProject(t *testing.T) {
	a, tt, asked := projectApp(t, t.TempDir())
	a.newProject()
	a.renaming = ""
	p := a.projects[0]
	p.Name, p.Instructions = "Docs", "Write plainly."
	a.newChat()
	a.send("A question")
	th := a.current()
	tt.Frame()
	if err := tt.Click(locales.More()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.MoveToProject(), "Docs"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Frame()
	if th.Project != p.ID {
		t.Fatalf("project %q", th.Project)
	}
	a.send("Another")
	if got := (*asked)[len(*asked)-1]; got[0].Content != "Write plainly." {
		t.Errorf("asked %+v", got)
	}
	if err := tt.Click(locales.More()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.MoveToProject(), locales.RemoveFromProject()); err != nil {
		t.Fatal(err)
	}
	if th.Project != "" {
		t.Errorf("still in %q", th.Project)
	}
}

// Removing a project asks first, then keeps its chats, out of any project;
// its page shows a new chat. The projects and the chats' are kept for the
// next start.
func TestRemoveProject(t *testing.T) {
	dir := t.TempDir()
	a, tt, _ := projectApp(t, dir)
	a.newProject()
	a.renaming = ""
	p := a.projects[0]
	p.Name, p.Instructions = "Old", "Be brief."
	a.saveProjects()
	a.send("Kept")
	th := a.current()
	a.open(projectPrefix + p.ID)
	tt.Frame()

	// Kept for the next start, first.
	a.saver.wait()
	a.projectsSaver.wait()
	again := newApp(&counter{})
	again.loadFrom(dir)
	if len(again.projects) != 1 || again.projects[0].Instructions != "Be brief." || again.threads[0].Project != p.ID {
		t.Fatalf("kept: projects %+v, chat's %q", again.projects, again.threads[0].Project)
	}

	if err := tt.Click(locales.More()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.RemoveProjectItem()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.RemoveProjectTitle("Old")) || !tt.HasText(locales.RemoveProjectAbout()) {
		t.Fatalf("no alert in %q", tt.Texts())
	}
	if err := tt.Click(locales.RemoveProjectButton()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.projects) != 0 || th.Project != "" || a.selected != "" || !slices.Contains(a.threads, th) {
		t.Errorf("projects %d, chat's %q, shown %q", len(a.projects), th.Project, a.selected)
	}
	a.saver.wait()
	a.projectsSaver.wait()
	again = newApp(&counter{})
	again.loadFrom(dir)
	if len(again.projects) != 0 || again.threads[0].Project != "" {
		t.Errorf("removed, kept: projects %d, chat's %q", len(again.projects), again.threads[0].Project)
	}
}

// A project's chats show under its row, folded at first, unfolded by its
// chevron, the latest five and the one shown, View all where there are
// more; and nowhere else, but pinned, in Pinned alone. layout.json keeps
// which are unfolded.
func TestProjectNesting(t *testing.T) {
	dir := t.TempDir()
	a, tt, _ := projectApp(t, dir)
	a.newProject()
	a.renaming = ""
	p := a.projects[0]
	p.Name = "Work"
	var chats []*thread
	for i := range 7 {
		a.newChat()
		a.send("Question " + string(rune('A'+i)))
		th := a.current()
		th.Project = p.ID
		th.Updated = time.Now().Add(time.Duration(i) * time.Minute)
		chats = append(chats, th)
	}
	a.newChat()
	a.foldProject(p.ID, true) // folded, as a project is at first
	a.rev++
	tt.Frame()
	nested := func() []string {
		var out []string
		for _, r := range a.chatRows(time.Now()) {
			switch {
			case r.nested:
				out = append(out, r.th.Title)
			case r.viewAll != nil:
				out = append(out, "View all")
			}
		}
		return out
	}
	if got := nested(); len(got) != 0 || slices.Contains(sidebar(a), "Question A") {
		t.Fatalf("folded: nested %q, chats %q", got, sidebar(a))
	}

	// Its chevron, under the pointer, unfolds it.
	r, _ := tt.Find("Work")
	tt.Move(r.X+30, r.Y+r.H/2)
	tt.Advance(time.Second)
	if err := tt.Click(locales.ProjectChatsToggle("Work")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want := []string{"Question G", "Question F", "Question E", "Question D", "Question C", "View all"}
	if got := nested(); !slices.Equal(got, want) {
		t.Errorf("unfolded: %q, want %q", got, want)
	}
	if slices.ContainsFunc(sidebar(a), func(s string) bool { return strings.HasPrefix(s, "Question") }) {
		t.Errorf("in Chats too: %q", sidebar(a))
	}

	// The oldest, shown, shows too.
	a.open(strconv.Itoa(chats[0].ID))
	tt.Frame()
	if got := nested(); !slices.Equal(got, []string{"Question G", "Question F", "Question E", "Question D", "Question C", "Question A", "View all"}) {
		t.Errorf("the oldest shown: %q", got)
	}
	// View all shows the page.
	if err := tt.Click(locales.ViewAll()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != projectPrefix+p.ID {
		t.Errorf("View all shows %q", a.selected)
	}

	// Kept unfolded.
	a.layoutSaver.wait()
	again := newApp(&counter{})
	again.loadFrom(dir)
	if !again.openProjects[p.ID] {
		t.Error("not kept unfolded")
	}

	// Pinned, in Pinned alone, its project by its model.
	a.togglePin(chats[6])
	tt.Advance(time.Second)
	if got := nested(); slices.Contains(got, "Question G") || !slices.Contains(sidebar(a), "Question G") || !tt.HasText("Work · counter") {
		t.Errorf("pinned: nested %q, sidebar %q", got, sidebar(a))
	}

	// Folded again by its chevron.
	tt.Move(r.X+30, r.Y+r.H/2)
	tt.Advance(time.Second)
	if err := tt.Click(locales.ProjectChatsToggle("Work")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := nested(); len(got) != 0 {
		t.Errorf("folded again: %q", got)
	}
}
