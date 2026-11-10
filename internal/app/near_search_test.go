// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bufio"
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/search"
)

func quickTitles(u *user, text string) []string {
	u.e.t.Helper()
	out := []string{}
	for _, row := range u.ok("GET", "/api/v1/search/quick?q="+url.QueryEscape(text), nil)["pages"].([]any) {
		out = append(out, row.(map[string]any)["title"].(string))
	}
	return out
}

func quickFiles(u *user, text string) []string {
	u.e.t.Helper()
	out := []string{}
	for _, row := range u.ok("GET", "/api/v1/search/quick?q="+url.QueryEscape(text), nil)["files"].([]any) {
		out = append(out, row.(map[string]any)["filename"].(string))
	}
	return out
}

func fullTitles(u *user, text string) []string {
	u.e.t.Helper()
	out := []string{}
	for _, row := range u.ok("GET", "/api/v1/search?q="+url.QueryEscape(text), nil)["results"].([]any) {
		out = append(out, row.(map[string]any)["title"].(string))
	}
	return out
}

func screenFiles(u *user, space, name string) []string {
	u.e.t.Helper()
	out := []string{}
	for _, row := range u.ok("GET", "/api/v1/spaces/"+space+"/files?q="+url.QueryEscape(name), nil)["files"].([]any) {
		out = append(out, row.(map[string]any)["filename"].(string))
	}
	return out
}

func TestANearMatchNeverShowsWhatTheReaderMayNotSee(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	closed, _ := owner.space("HR")
	hidden := owner.page(space, nil, "Quarterly report", "x")
	owner.restrict(hidden, "view", owner.id)
	owner.page(closed, nil, "Quarterly forecast", "x")
	owner.file(hidden, "budget-forecast.txt", "x")

	for _, typed := range []string{"quartely", "uarterl"} {
		if got := quickTitles(viewer, typed); len(got) != 0 {
			t.Fatalf("the box showed %v for %q", got, typed)
		}
		if got := fullTitles(viewer, typed); len(got) != 0 {
			t.Fatalf("search showed %v for %q", got, typed)
		}
	}
	if got := quickFiles(viewer, "forcast"); len(got) != 0 {
		t.Fatalf("the box showed the file of a hidden page: %v", got)
	}
	if got := screenFiles(viewer, space, "forcast"); len(got) != 0 {
		t.Fatalf("the Files screen showed the file of a hidden page: %v", got)
	}
	if got := quickTitles(owner, "quartely"); !has(got, "Quarterly report") || !has(got, "Quarterly forecast") {
		t.Fatalf("the owner, who may see both: %v", got)
	}

	owner.restrict(hidden, "view")
	if got := quickTitles(viewer, "quartely"); !has(got, "Quarterly report") || has(got, "Quarterly forecast") {
		t.Fatalf("once open: %v", got)
	}
	owner.restrict(hidden, "view", owner.id)
	if got := quickTitles(viewer, "quartely"); len(got) != 0 {
		t.Fatalf("a page closed a moment ago: %v", got)
	}
}

func TestTitlesThatHoldTheTypedTextOrANearWordComeAfterTheWords(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	for _, title := range []string{"Budgte", "Rebudgeting", "Budget plan", "Budget", "Unrelated"} {
		owner.page(space, nil, title, "x")
	}
	owner.page(space, nil, "Notes", "the budget of the team")

	// In the box a typo in a title stands before a word in the text.
	want := []string{"Budget", "Budget plan", "Rebudgeting", "Budgte", "Notes"}
	if got := quickTitles(owner, "budget"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the box: %v, want %v", got, want)
	}
	// In a full search a real word in the text stands before a title with a typo (D13).
	want = []string{"Budget", "Budget plan", "Rebudgeting", "Notes", "Budgte"}
	if got := fullTitles(owner, "budget"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("search: %v, want %v", got, want)
	}
}

func TestTheNearestTitleComesFirstAndShortTypingFindsNoNearMatch(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	owner.page(space, nil, "Handbook", "x")
	owner.page(space, nil, "Handbag", "x")
	if got := quickTitles(owner, "handbok"); len(got) == 0 || got[0] != "Handbook" {
		t.Fatalf("the closest first: %v", got)
	}
	// "nd" is inside "Handbook", but two letters are too few to look inside words.
	if got := quickTitles(owner, "nd"); len(got) != 0 {
		t.Fatalf("two letters found %v", got)
	}
	// Four letters that share only their start with a title are no typo of it.
	owner.page(space, nil, "Welcome pack", "x")
	if got := quickTitles(owner, "week"); len(got) != 0 {
		t.Fatalf("four letters found a typo: %v", got)
	}
	if got := quickTitles(owner, `"handbok"`); len(got) != 0 {
		t.Fatalf("a quoted search found near matches: %v", got)
	}
}

func TestPercentAndUnderscoreAreTakenAsLetters(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	owner.page(space, nil, "plan_a", "x")
	owner.page(space, nil, "planxa", "x")
	if got := quickTitles(owner, "n_a"); !has(got, "plan_a") || has(got, "planxa") {
		t.Fatalf("an underscore matched any letter: %v", got)
	}
}

func TestAFileNameIsFoundTheSameWayInTheBoxAndOnTheFilesScreen(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	for _, name := range []string{"forecast.txt", "budget-forecast.txt", "weatherforecasts.txt", "forcast-old.txt", "minutes.txt"} {
		owner.file(page, name, "x")
	}
	box := quickFiles(owner, "forecast")
	screen := screenFiles(owner, space, "forecast")
	if strings.Join(box[:3], "|") != strings.Join(screen[:3], "|") {
		t.Fatalf("the two disagree: %v and %v", box, screen)
	}
	if !has(screen, "weatherforecasts.txt") || !has(screen, "forcast-old.txt") || has(screen, "minutes.txt") {
		t.Fatalf("the Files screen: %v", screen)
	}
	if last := screen[len(screen)-1]; last != "forcast-old.txt" {
		t.Fatalf("a typo comes after the words and the names that hold them: %v", screen)
	}
	if got := quickFiles(owner, "forcast"); !has(got, "forecast.txt") {
		t.Fatalf("a typo in the box: %v", got)
	}
}

func TestTheThresholdFindsTheTyposAndLeavesTheDecoys(t *testing.T) {
	e := newEnv(t)
	threshold, err := strconv.ParseFloat(search.NearThreshold, 64)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../search/testdata/typos.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	counts := map[string]int{}
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		line := lines.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		var score float64
		if err := e.app.Pool().QueryRow(context.Background(), "SELECT word_similarity(lower($1), lower($2))", parts[1], parts[2]).Scan(&score); err != nil {
			t.Fatal(err)
		}
		counts[parts[0]]++
		if found := score >= threshold; found != (parts[0] == "find") {
			t.Errorf("%s %q for %q: %.3f against %.2f", parts[0], parts[1], parts[2], score, threshold)
		}
	}
	if counts["find"] < 40 || counts["decoy"] < 20 {
		t.Fatalf("the set is too small: %v", counts)
	}
}
