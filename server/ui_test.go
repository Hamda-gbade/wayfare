package server

import (
	"regexp"
	"strings"
	"testing"
)

// TestUIPreservesContentOnFetchError pins the contract from issue #293: a
// failed fetch must never empty the page. Previously measure() and
// loadTrend() cleared #out before the request, so an error left the reader
// staring at an empty page with nothing to recover, forcing them to re-enter
// everything. Now the last successful render stays visible and the error is
// shown as a banner above it.
//
// These assertions are on the UI source because the page is a single
// embedded file with no build step and no JS test runner in CI — the same
// approach as TestUITrendIsSelfContained.
func TestUIPreservesContentOnFetchError(t *testing.T) {
	raw, err := uiFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	// Nothing may blank #out before a fetch resolves. The only assignments
	// to #out.innerHTML allowed are writes of freshly rendered content.
	if strings.Contains(page, "$('out').innerHTML = '';") {
		t.Error("#out is cleared before a fetch; a failed request would leave an empty page")
	}

	for _, want := range []string{
		"function showErrorBanner",        // the shared preserve-and-banner path
		"insertAdjacentHTML('afterbegin'", // the banner goes above retained content
		`role="alert"`,                    // announced to assistive tech, not colour alone
		"prior.remove()",                  // retrying never stacks banners
		"Could not measure:",              // the measure failure keeps its text
		"Could not load history:",         // the trend failure keeps its text
		"Could not load corridor data:",   // the corridor failure keeps its text
	} {
		if !strings.Contains(page, want) {
			t.Errorf("UI missing %q; an error would not preserve and explain the page", want)
		}
	}

	// A failed history load must not drop the last good trend state: the
	// only remaining assignment is the declaration itself.
	if n := strings.Count(page, "trendState = null"); n != 1 {
		t.Errorf(`found %d "trendState = null" assignments, want 1 (the declaration only); a failed trend load must keep the previous trend readable`, n)
	}

	// Definition plus at least the three fetch paths route errors through
	// the banner helper rather than replacing #out.
	if n := strings.Count(page, "showErrorBanner("); n < 4 {
		t.Errorf("showErrorBanner used %d times, want at least 4 (definition + three fetch paths)", n)
	}
}

// TestUIStateIsNotCarriedByColourAlone pins the contract from issue #304: a
// state may not be carried by hue. A verdict used to be a `.v-good` /
// `.v-poor` / `.v-unusable` colour class and nothing else, so a greyscale
// print, a forced-colors mode or a reader who does not separate those hues
// lost the grade along with the colour.
//
// These assertions are on the UI source for the reason TestUIPreservesContentOnFetchError
// gives: the page is one embedded file with no build step and no JS test runner
// in CI, so this can only prove the vocabulary is present and cannot prove how
// it renders. What it renders like is measured in a browser by
// docs/qa/browser/run-state-legibility.mjs, which drives the real binary
// through every state, both schemes and five widths.
func TestUIStateIsNotCarriedByColourAlone(t *testing.T) {
	raw, err := uiFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	// Every grade the engine publishes needs a mark, and the marks have to be
	// different from one another: two grades printing the same shape are one
	// grade to a reader without colour. A mark for a grade the engine does not
	// publish would be worse — it would be dressing a verdict this client has
	// never seen — so the key set is pinned to the four.
	block := regexp.MustCompile(`VERDICT_MARKS = \{([^}]*)\}`).FindStringSubmatch(page)
	if block == nil {
		t.Fatal("UI has no VERDICT_MARKS table; a verdict would be carried by its colour class alone")
	}
	grades := regexp.MustCompile(`([A-Z]+):\s*'([^']*)'`).FindAllStringSubmatch(block[1], -1)
	if len(grades) != 4 {
		t.Errorf("VERDICT_MARKS holds %d grades, want the 4 the engine publishes (GOOD, FAIR, POOR, UNUSABLE)", len(grades))
	}
	seen := map[string]string{}
	for _, g := range grades {
		grade, mark := g[1], g[2]
		switch grade {
		case "GOOD", "FAIR", "POOR", "UNUSABLE":
		default:
			t.Errorf("VERDICT_MARKS has a mark for %q, a grade the engine does not publish", grade)
		}
		if mark == "" {
			t.Errorf("VERDICT_MARKS[%s] is empty; that grade would be carried by colour alone", grade)
		}
		if other, dup := seen[mark]; dup {
			t.Errorf("grades %s and %s share the mark %q; without colour they are one state", other, grade, mark)
		}
		seen[mark] = grade
	}

	// The mark is decoration in front of the grade: assistive tech must read
	// the grade once, and the chip must not be announced as part of it.
	if !strings.Contains(page, `<span class="v-mark" aria-hidden="true">`) {
		t.Error("a verdict mark is not aria-hidden; the grade would be announced twice")
	}

	// Every place a verdict is rendered has to carry the mark, not only the
	// table: the threshold legend, the recommendation line and the table cell.
	// Definition plus those three call sites.
	if n := strings.Count(page, "verdictMark("); n < 4 {
		t.Errorf("verdictMark used %d times, want at least 4 (definition, table cell, threshold legend, recommendation)", n)
	}
	for _, want := range []string{"verdictMark('GOOD')", "verdictMark('FAIR')", "verdictMark('POOR')", "verdictMark('UNUSABLE')"} {
		if !strings.Contains(page, want) {
			t.Errorf("UI missing %q; that grade would reach the reader as a word inside a colour class", want)
		}
	}

	// A mark that wraps onto its own line is a mark a line away from the word
	// it names, which at 320px is exactly what a narrow verdict column does.
	if !strings.Contains(page, ".v-cell { white-space: nowrap; }") {
		t.Error("the verdict cell does not hold the mark and its grade on one line")
	}

	// The three check states already had marks; keep them, and give the
	// undetermined metric the same one, so "could not determine" reads as one
	// idea wherever it appears.
	for _, want := range []string{
		`.f-pass::before { content: "✓"; }`,
		`.f-fail::before { content: "×"; }`,
		`.f-unknown::before { content: "?"; }`,
		`.m-state::before { content: "?";`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("UI missing %q; that state has no mark of its own", want)
		}
	}

	// Live and recorded are told apart by the words in the banner already, so
	// the border is a second channel rather than the only one: dashed for a
	// recorded reading, solid for a live one.
	if !strings.Contains(page, "border: 1px dashed var(--warn); color: var(--warn); background: color-mix(in srgb, var(--warn) 10%, transparent); }") {
		t.Error("the recorded-reading banner is not distinguished by a dashed edge")
	}
	if !strings.Contains(page, "border-color: var(--ok); border-style: solid;") {
		t.Error("the live-measurement banner is not distinguished by a solid edge")
	}

	// A run's integrity state in the trend plot was a dot's fill, which is the
	// only channel there was. It now has a shape per state, and the key draws
	// those shapes rather than colour swatches. The key is a named group, so it
	// is announced as one thing rather than as three loose words.
	for _, want := range []string{
		"const dotMark = (integrity, cx, cy, r, attrs, title)",
		"if (integrity === 'DERIVATIVE')",
		"if (integrity === 'NO-MARKET')",
		`class="run-mark"`,
		`class="key-mark"`,
		`role="group" aria-label="Integrity state key"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("UI missing %q; a stored run's integrity state is still carried by its fill alone", want)
		}
	}

	// Both tables are wider than a phone, and at 320px the verdict column is
	// past the right edge. A scrollable region that cannot take focus is
	// reachable by pointer only, so the grade would be too.
	if n := strings.Count(page, `class="scroll" tabindex="0"`); n != 2 {
		t.Errorf("found %d focusable scroll regions, want 2 (the measurements table and the stored-runs table)", n)
	}
	if n := strings.Count(page, `role="region"`); n != 2 {
		t.Errorf("found %d named scroll regions, want 2; an unnamed region is announced as nothing", n)
	}
}
