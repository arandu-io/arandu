// Example resource. Remove with the list under "The example resource" in README.md.

package unit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	hhttp "github.com/arandu-io/hesape/http"

	resources "github.com/arandu-io/arandu/app/Http/Resources"
	models "github.com/arandu-io/arandu/app/Models"
)

// TestTheNoteResourceAnswersOnlyWhatItLists writes one Note through
// ctx.JSON, as a controller does, and reads the keys of what was answered.
// They have to be the allow-list and nothing more -- the tenant
// in particular, which is set on the record and must not leave.
func TestTheNoteResourceAnswersOnlyWhatItLists(t *testing.T) {
	record := &models.Note{ID: "record-1", TenantID: "tenant-a"}

	// No server and no router: ctx.JSON is called on a context over a
	// recorder, the way the router would call it, with a plain request.
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	ctx := hhttp.NewContext(w, request, nil, nil)
	if err := ctx.JSON(http.StatusOK, resources.NewNoteResource(record)); err != nil {
		t.Fatalf("ctx.JSON: %v", err)
	}

	var answer struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	answered := make([]string, 0, len(answer.Data))
	for key := range answer.Data {
		answered = append(answered, key)
	}
	sort.Strings(answered)

	// The allow-list, as ToArray writes it. A field added there is added here,
	// on purpose: what leaves is decided twice, in two files, by somebody
	// reading both.
	listed := []string{"body", "created_at", "id", "pinned", "published_at", "title", "updated_at", "user_id"}
	if strings.Join(answered, ",") != strings.Join(listed, ",") {
		t.Errorf("answered %v, want exactly %v", answered, listed)
	}
	if answer.Data["id"] != "record-1" {
		t.Errorf("id = %v, want the record's", answer.Data["id"])
	}
}

// arandu:begin custom
// arandu:end custom
