// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/test"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/support"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// foreignTenant is a tenant that is not the application's, and whose rows no
// listing of this tenant may ever show, count or offer.
const foreignTenant = "22222222-2222-4222-8222-222222222222"

var (
	// listedTitles reads the title cell of every row of a table, in order.
	listedTitles = regexp.MustCompile(`(?s)data-column="title"[^>]*>\s*<a\s[^>]*>\s*(.*?)\s*</a>`)
	// listedKeys reads the key of every row.
	listedKeys = regexp.MustCompile(`data-part="row" data-key="([^"]+)"`)
	// listedTotal is the number of rows the whole list holds, as the table
	// writes it for the script.
	listedTotal = regexp.MustCompile(`data-total="(\d+)"`)
	// pageOfText is the "Page X of Y" the pager says.
	pageOfText = regexp.MustCompile(`data-page-of-template="[^"]*"\s*>([^<]*)<`)
)

// tagWith is the opening tag of the first element whose attributes include
// marker, or empty when there is none. The markup writes an attribute per
// line, in an order of its own, so a test asks about one tag and never about
// the text around it.
func tagWith(page, marker string) string {
	return regexp.MustCompile(`<[a-z]+\s[^>]*` + regexp.QuoteMeta(marker) + `[^>]*>`).FindString(page)
}

// titlesOn is the titles of the rows a page lists, in the order it lists them.
func titlesOn(page string) []string {
	var out []string
	for _, match := range listedTitles.FindAllStringSubmatch(page, -1) {
		out = append(out, html.UnescapeString(match[1]))
	}
	return out
}

// facetCountOn is the count the table writes beside one value of a facet.
func facetCountOn(t *testing.T, page, param, value string) int {
	t.Helper()
	pattern := regexp.MustCompile(`(?s)name="` + param + `"\s+value="` + value + `"[^>]*>\s*<span[^>]*>[^<]*</span>\s*<span[^>]*>(\d+)</span>`)
	match := pattern.FindStringSubmatch(page)
	if match == nil {
		t.Fatalf("the table writes no count beside %s=%s", param, value)
	}
	count, _ := strconv.Atoi(match[1])
	return count
}

// totalOn is how many rows the whole list holds.
func totalOn(t *testing.T, page string) int {
	t.Helper()
	match := listedTotal.FindStringSubmatch(page)
	if match == nil {
		t.Fatal("the table writes no total")
	}
	total, _ := strconv.Atoi(match[1])
	return total
}

// seedNote is one note of a listing fixture.
type seedNote struct {
	title, body string
	pinned      bool
	// author is the member who wrote it, and tenant the tenant it belongs to;
	// the first member and the application's tenant when empty.
	author, tenant string
}

// seed stores the notes one minute apart, so that "newest first" is the
// reverse of the order they are given in.
func (f notesFixture) seed(t *testing.T, notes ...seedNote) {
	t.Helper()
	support.FreezeTime()
	t.Cleanup(support.TravelBack)
	for _, n := range notes {
		author, tenant := n.author, n.tenant
		if author == "" {
			author = f.anaID
		}
		if tenant == "" {
			tenant = appconfig.Tenant()
		}
		if _, err := factories.Notes(f.app.DB).State(func(row *models.Note) {
			row.UserID, row.Title, row.Body, row.Pinned = author, n.title, n.body, n.pinned
		}).CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, tenant)); err != nil {
			t.Fatalf("creating the note %q: %v", n.title, err)
		}
		support.Travel(time.Minute)
	}
}

// fiveNotes is the fixture most of the listing tests read: three pinned and
// two not, with words shared between titles and bodies, and one note of a
// tenant that must never be seen.
func (f notesFixture) fiveNotes(t *testing.T) {
	t.Helper()
	f.seed(t,
		seedNote{title: "Alpha harbour", body: "ships and sails", pinned: true},
		seedNote{title: "Bravo market", body: "bread and cheese"},
		seedNote{title: "Charlie garden", body: "tomatoes and SHIPS", pinned: true, author: f.beaID},
		seedNote{title: "Delta workshop", body: "tools"},
		seedNote{title: "Echo studio", body: "paint and canvas", pinned: true, author: f.beaID},
		seedNote{title: "Zulu foreign", body: "ships belong to nobody here", pinned: true, tenant: foreignTenant},
	)
}

// TestTheListingIsNewestFirstUntilAskedOtherwise: the default order is the
// listing's declared one, and a rendered page is the whole tenant's notes and
// nobody else's.
func TestTheListingIsNewestFirstUntilAskedOtherwise(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)

	page := f.ana.Get("/notes").AssertOk().GetContent()
	want := []string{"Echo studio", "Delta workshop", "Charlie garden", "Bravo market", "Alpha harbour"}
	if got := titlesOn(page); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
	if got := totalOn(t, page); got != 5 {
		t.Errorf("total = %d, want the 5 notes of the tenant", got)
	}
	// Another member of the tenant reads the same list.
	if got := titlesOn(f.bea.Get("/notes").AssertOk().GetContent()); !slices.Equal(got, want) {
		t.Errorf("the other member's titles = %v, want %v", got, want)
	}
}

// TestTheListingSearchesTitleAndBodyWithoutCase: the text is looked for as
// "contains" in the declared columns, whatever its case, and a wildcard is
// looked for as itself.
func TestTheListingSearchesTitleAndBodyWithoutCase(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"q=ships", []string{"Charlie garden", "Alpha harbour"}},
		{"q=SHIPS", []string{"Charlie garden", "Alpha harbour"}},
		{"q=Harb", []string{"Alpha harbour"}},
		{"q=" + url.QueryEscape("  market  "), []string{"Bravo market"}},
		{"q=%25", nil},
		{"q=_", nil},
		{"q=nothing+matches", nil},
		{"q=zulu", nil},
	} {
		page := f.ana.Get("/notes?" + tc.query).AssertOk().GetContent()
		if got := titlesOn(page); !slices.Equal(got, tc.want) {
			t.Errorf("?%s: titles = %v, want %v", tc.query, got, tc.want)
		}
		if len(tc.want) == 0 && !strings.Contains(page, "No results.") {
			t.Errorf("?%s matched nothing and the table does not say so", tc.query)
		}
	}

	// The text is in the box again, and the page links keep it.
	page := f.ana.Get("/notes?q=ships").AssertOk().GetContent()
	if !strings.Contains(page, `value="ships"`) {
		t.Error("the search box does not hold the text that was searched")
	}
}

// TestTheListingIsOrderedByTheDeclaredColumns: every sortable column orders
// both ways, and the order is stable.
func TestTheListingIsOrderedByTheDeclaredColumns(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"sort=title", []string{"Alpha harbour", "Bravo market", "Charlie garden", "Delta workshop", "Echo studio"}},
		{"sort=title&dir=asc", []string{"Alpha harbour", "Bravo market", "Charlie garden", "Delta workshop", "Echo studio"}},
		{"sort=title&dir=desc", []string{"Echo studio", "Delta workshop", "Charlie garden", "Bravo market", "Alpha harbour"}},
		{"sort=body", []string{"Bravo market", "Echo studio", "Alpha harbour", "Charlie garden", "Delta workshop"}},
		{"sort=created_at&dir=asc", []string{"Alpha harbour", "Bravo market", "Charlie garden", "Delta workshop", "Echo studio"}},
		{"sort=created_at&dir=desc", []string{"Echo studio", "Delta workshop", "Charlie garden", "Bravo market", "Alpha harbour"}},
	} {
		page := f.ana.Get("/notes?" + tc.query).AssertOk().GetContent()
		if got := titlesOn(page); !slices.Equal(got, tc.want) {
			t.Errorf("?%s: titles = %v, want %v", tc.query, got, tc.want)
		}
	}

	// A boolean orders as a boolean: the unpinned group and the pinned group
	// are whole, one after the other, in the direction asked for. Inside a
	// group the tie-break decides, and it is the listing's.
	unpinned := []string{"Bravo market", "Delta workshop"}
	pinned := []string{"Alpha harbour", "Charlie garden", "Echo studio"}
	asc := titlesOn(f.ana.Get("/notes?sort=pinned&dir=asc").AssertOk().GetContent())
	if len(asc) != 5 || !sameSet(asc[:2], unpinned) || !sameSet(asc[2:], pinned) {
		t.Errorf("sort=pinned&dir=asc: titles = %v, want the unpinned notes and then the pinned ones", asc)
	}
	desc := titlesOn(f.ana.Get("/notes?sort=pinned&dir=desc").AssertOk().GetContent())
	if len(desc) != 5 || !sameSet(desc[:3], pinned) || !sameSet(desc[3:], unpinned) {
		t.Errorf("sort=pinned&dir=desc: titles = %v, want the pinned notes and then the unpinned ones", desc)
	}
}

// sameSet reports whether two lists hold the same titles, in any order.
func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// TestTheFacetNarrowsAndCountsTheWayTheReferenceDoes: choosing a value keeps
// the rows that carry it, choosing both is the union, and the count beside a
// value ignores the facet's own choice and respects the search.
func TestTheFacetNarrowsAndCountsTheWayTheReferenceDoes(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)

	pinned := f.ana.Get("/notes?pinned=1").AssertOk().GetContent()
	if got, want := titlesOn(pinned), []string{"Echo studio", "Charlie garden", "Alpha harbour"}; !slices.Equal(got, want) {
		t.Errorf("pinned=1: titles = %v, want %v", got, want)
	}
	// Both counts are of the tenant's whole list: the choice of this facet does
	// not hide what the other value would add, and the foreign pinned note is
	// in neither.
	if on, off := facetCountOn(t, pinned, "pinned", "1"), facetCountOn(t, pinned, "pinned", "0"); on != 3 || off != 2 {
		t.Errorf("counts with pinned=1 = %d pinned and %d not, want 3 and 2", on, off)
	}

	unpinned := f.ana.Get("/notes?pinned=0").AssertOk().GetContent()
	if got, want := titlesOn(unpinned), []string{"Delta workshop", "Bravo market"}; !slices.Equal(got, want) {
		t.Errorf("pinned=0: titles = %v, want %v", got, want)
	}

	both := f.ana.Get("/notes?pinned=1&pinned=0").AssertOk().GetContent()
	if got := totalOn(t, both); got != 5 {
		t.Errorf("both values: total = %d, want 5", got)
	}

	// The search narrows the counts, the facet's own choice does not.
	searched := f.ana.Get("/notes?q=ships&pinned=0").AssertOk().GetContent()
	if got := titlesOn(searched); len(got) != 0 {
		t.Errorf("q=ships&pinned=0: titles = %v, want none", got)
	}
	if on, off := facetCountOn(t, searched, "pinned", "1"), facetCountOn(t, searched, "pinned", "0"); on != 2 || off != 0 {
		t.Errorf("counts with q=ships = %d pinned and %d not, want 2 and 0", on, off)
	}

	// A value the facet does not declare is dropped, and the list is whole.
	unknown := f.ana.Get("/notes?pinned=2&pinned=true").AssertOk().GetContent()
	if got := totalOn(t, unknown); got != 5 {
		t.Errorf("undeclared facet values: total = %d, want 5", got)
	}
}

// TestTheListingPagesAndSizesThePage: the declared sizes are the only sizes,
// the page is a whole number inside the list, and a page past the end is the
// last one.
func TestTheListingPagesAndSizesThePage(t *testing.T) {
	f := newNotesFixture(t)
	notes := make([]seedNote, 0, 30)
	for i := 1; i <= 30; i++ {
		notes = append(notes, seedNote{title: fmt.Sprintf("Note %02d", i), body: "x"})
	}
	f.seed(t, notes...)

	first := f.ana.Get("/notes?sort=title").AssertOk().GetContent()
	if got := titlesOn(first); len(got) != 25 || got[0] != "Note 01" || got[24] != "Note 25" {
		t.Errorf("the default page = %d rows from %q, want the first 25", len(got), got)
	}
	if got := pageOfText.FindStringSubmatch(first); got == nil || strings.TrimSpace(got[1]) != "Page 1 of 2" {
		t.Errorf("pager says %v, want Page 1 of 2", got)
	}

	for _, tc := range []struct {
		query       string
		from, to    string
		rows        int
		pageOf      string
		description string
	}{
		{"sort=title&per_page=10", "Note 01", "Note 10", 10, "Page 1 of 3", "a declared size"},
		{"sort=title&per_page=10&page=3", "Note 21", "Note 30", 10, "Page 3 of 3", "the last page"},
		{"sort=title&per_page=10&page=4", "Note 21", "Note 30", 10, "Page 3 of 3", "a page past the end is the last"},
		{"sort=title&per_page=10&page=99999999999999999999", "Note 01", "Note 10", 10, "Page 1 of 3", "a page that is no int is the first"},
		{"sort=title&per_page=50", "Note 01", "Note 30", 30, "", "the largest size, with no second page to say anything about"},
		{"sort=title&per_page=25&page=2", "Note 26", "Note 30", 5, "Page 2 of 2", "the second page"},
		{"sort=title&per_page=100", "Note 01", "Note 25", 25, "Page 1 of 2", "a size nobody declared is the default"},
		{"sort=title&per_page=1000000&page=2", "Note 26", "Note 30", 5, "Page 2 of 2", "a size past every declared one is the default"},
		{"sort=title&per_page=-5", "Note 01", "Note 25", 25, "Page 1 of 2", "a size below one is the default"},
	} {
		page := f.ana.Get("/notes?" + tc.query).AssertOk().GetContent()
		got := titlesOn(page)
		if len(got) != tc.rows || got[0] != tc.from || got[len(got)-1] != tc.to {
			t.Errorf("?%s (%s): got %v, want %d rows from %q to %q", tc.query, tc.description, got, tc.rows, tc.from, tc.to)
		}
		said := pageOfText.FindStringSubmatch(page)
		switch {
		case tc.pageOf == "" && said != nil:
			t.Errorf("?%s (%s): a list of one page says %q", tc.query, tc.description, said[1])
		case tc.pageOf != "" && (said == nil || strings.TrimSpace(said[1]) != tc.pageOf):
			t.Errorf("?%s (%s): pager says %v, want %s", tc.query, tc.description, said, tc.pageOf)
		}
	}
}

// TestTheHiddenColumnsTravelInTheAddress: a column the person hid is hidden
// in the answer, in every control's address and in the form, and only a
// column the listing declared hideable can be.
func TestTheHiddenColumnsTravelInTheAddress(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)

	page := f.ana.Get("/notes?hide=body&hide=created_at").AssertOk().GetContent()
	for _, column := range []string{"body", "created_at"} {
		if !regexp.MustCompile(`data-column="` + column + `"[^>]*\bhidden\b`).MatchString(page) {
			t.Errorf("hide=%s does not hide the column", column)
		}
	}
	for _, column := range []string{"title", "pinned"} {
		if regexp.MustCompile(`data-column="` + column + `"[^>]*\bhidden\b`).MatchString(page) {
			t.Errorf("the column %s is hidden and the address did not ask for it", column)
		}
	}
	// A step keeps what is hidden: the sort and the form both carry it.
	for _, want := range []string{`name="hide" value="body"`, `name="hide" value="created_at"`, `hide=body&amp;hide=created_at`} {
		if !strings.Contains(page, want) {
			t.Errorf("the table does not carry %s on its controls", want)
		}
	}

	// The title is not hideable, and nor is anything the listing did not name.
	page = f.ana.Get("/notes?hide=title&hide=password&hide=").AssertOk().GetContent()
	if regexp.MustCompile(`data-column="title"[^>]*\bhidden\b`).MatchString(page) || strings.Contains(page, `name="hide"`) {
		t.Error("a column the listing did not declare hideable was hidden")
	}
}

// TestAHostileListingQueryFallsBackToTheDefaults: nothing in the query string
// fails the request or reaches the statement. Each value is answered with the
// list the defaults give.
func TestAHostileListingQueryFallsBackToTheDefaults(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)
	defaults := titlesOn(f.ana.Get("/notes").AssertOk().GetContent())

	for _, query := range []string{
		"sort=password",
		"sort=" + url.QueryEscape("title; drop table notes"),
		"sort=" + url.QueryEscape("title desc, id"),
		"sort=" + url.QueryEscape("(select 1)"),
		"sort=tenant_id",
		"sort=TITLE",
		"sort=id",
		"sort[]=title",
		"dir=sideways",
		"dir=" + url.QueryEscape("desc; --"),
		"per_page=1000000",
		"per_page=-1",
		"per_page=0",
		"per_page=abc",
		"per_page=" + url.QueryEscape("10 or 1=1"),
		"page=0",
		"page=-3",
		"page=abc",
		"page=1e9",
		"pinned=" + url.QueryEscape("1 or 1=1"),
		"hide=" + url.QueryEscape("title"),
		"unknown=1&sort=nope&dir=nope&per_page=nope&page=nope",
	} {
		page := f.ana.Get("/notes?" + query).AssertOk().GetContent()
		if got := titlesOn(page); !slices.Equal(got, defaults) {
			t.Errorf("?%s: titles = %v, want the default %v", query, got, defaults)
		}
	}

	// A hostile sort next to a valid one does not hide the valid one.
	got := titlesOn(f.ana.Get("/notes?sort=title&sort=password&dir=desc").AssertOk().GetContent())
	if len(got) != 5 {
		t.Errorf("a repeated sort lost rows: %v", got)
	}
	// The long search is cut, not refused.
	f.ana.Get("/notes?q=" + strings.Repeat("a", 5000)).AssertOk()
}

// TestAnotherTenantsNotesNeverAppearInTheListing: whatever the query asks for,
// the tenant of the Grant is the only tenant read -- in the rows, in the total
// and in the facet counts.
func TestAnotherTenantsNotesNeverAppearInTheListing(t *testing.T) {
	f := newNotesFixture(t)
	f.fiveNotes(t)

	for _, query := range []string{
		"", "q=zulu", "q=foreign", "q=ships", "sort=title&dir=desc", "pinned=1", "pinned=1&per_page=50",
		"tenant_id=" + foreignTenant, "tenant=" + foreignTenant,
	} {
		page := f.ana.Get("/notes?" + query).AssertOk().GetContent()
		if strings.Contains(page, "Zulu foreign") {
			t.Errorf("?%s shows a note of another tenant", query)
		}
	}
	all := f.ana.Get("/notes?pinned=1&pinned=0&per_page=50").AssertOk().GetContent()
	if got := totalOn(t, all); got != 5 {
		t.Errorf("total = %d, want the 5 notes of the tenant", got)
	}
	if on := facetCountOn(t, all, "pinned", "1"); on != 3 {
		t.Errorf("pinned count = %d, want 3: the foreign pinned note was counted", on)
	}
	if got := f.ana.Get("/notes?q=zulu").AssertOk().GetContent(); totalOn(t, got) != 0 {
		t.Error("a search found the other tenant's note")
	}
}

// TestTheListingHasOnePager: the table draws its own pager, and no second
// pager sits beside it.
func TestTheListingHasOnePager(t *testing.T) {
	f := newNotesFixture(t)
	notes := make([]seedNote, 0, 30)
	for i := 1; i <= 30; i++ {
		notes = append(notes, seedNote{title: fmt.Sprintf("Note %02d", i), body: "x"})
	}
	f.seed(t, notes...)

	for _, address := range []string{"/notes", "/notes?page=2", "/notes?per_page=10"} {
		page := f.ana.Get(address).AssertOk().GetContent()
		if got := strings.Count(page, `data-part="pagination"`); got != 1 {
			t.Errorf("%s draws %d pagers of the table, want 1", address, got)
		}
		if strings.Contains(page, `rel="next"`) || strings.Contains(page, `rel="prev"`) || strings.Contains(page, `<nav data-part="root"`) {
			t.Errorf("%s draws a second pager next to the table's", address)
		}
	}
}

// TestTheJSONListingIsFilteredLikeThePage: a client that asked for JSON reads
// the same listing -- the same search, order, page and tenant -- and is given
// the next page's address with the state in it.
func TestTheJSONListingIsFilteredLikeThePage(t *testing.T) {
	f := newNotesFixture(t)
	notes := make([]seedNote, 0, 12)
	for i := 1; i <= 12; i++ {
		notes = append(notes, seedNote{title: fmt.Sprintf("Note %02d", i), body: "x"})
	}
	notes = append(notes, seedNote{title: "Note foreign", body: "x", tenant: foreignTenant})
	f.seed(t, notes...)

	answer := asJSON(f.ana, func(c *test.Client) *test.Response {
		return c.Get("/notes?sort=title&per_page=10&q=note")
	}).AssertOk()
	var body struct {
		Data struct {
			Notes []map[string]any `json:"notes"`
		} `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	decodeJSON(t, answer, "application/json", &body)
	if len(body.Data.Notes) != 10 || body.Data.Notes[0]["title"] != "Note 01" || body.Data.Notes[9]["title"] != "Note 10" {
		t.Errorf("notes = %d rows, want the first ten by title", len(body.Data.Notes))
	}
	next, err := url.Parse(body.Links.Next)
	if err != nil || next.Path != "/notes" {
		t.Fatalf("next = %q, want the listing's next page", body.Links.Next)
	}
	for key, want := range map[string]string{"page": "2", "per_page": "10", "q": "note", "sort": "title"} {
		if got := next.Query().Get(key); got != want {
			t.Errorf("next carries %s=%q, want %q", key, got, want)
		}
	}
	if !strings.Contains(answer.Header("Vary"), "Accept") {
		t.Errorf("Vary = %q, and the answer depends on Accept", answer.Header("Vary"))
	}

	second := asJSON(f.ana, func(c *test.Client) *test.Response { return c.Get(body.Links.Next) }).AssertOk()
	body.Data.Notes, body.Links.Next = nil, ""
	decodeJSON(t, second, "application/json", &body)
	if len(body.Data.Notes) != 2 || body.Links.Next != "" {
		t.Errorf("second page = %d rows with next %q, want the last two and no next", len(body.Data.Notes), body.Links.Next)
	}
	for _, row := range body.Data.Notes {
		if row["title"] == "Note foreign" {
			t.Error("the JSON listing carries a note of another tenant")
		}
	}
}

// TestNobodyReadsTheListingWithoutSigningIn: no query string is a way past the
// guard. The listing is the same route whatever it carries.
func TestNobodyReadsTheListingWithoutSigningIn(t *testing.T) {
	guest := tests.Serve(t).Client()
	for _, query := range []string{"", "?q=a", "?sort=title&dir=desc", "?pinned=1&per_page=50", "?sort=password"} {
		guest.Get("/notes" + query).AssertRedirect("/auth/login")
	}
}
