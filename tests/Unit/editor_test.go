package unit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorRecommendsTheAranduAndGoExtensions(t *testing.T) {
	path := filepath.Join("..", "..", ".vscode", "extensions.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read editor recommendations: %v", err)
	}

	var document struct {
		Recommendations []string `json:"recommendations"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse editor recommendations: %v", err)
	}

	want := map[string]bool{
		"arandu-io.arandu": false,
		"golang.go":        false,
	}
	for _, recommendation := range document.Recommendations {
		if _, ok := want[recommendation]; ok {
			want[recommendation] = true
		}
	}
	for extension, found := range want {
		if !found {
			t.Errorf("editor does not recommend %q", extension)
		}
	}
}

func TestKyseEditorSettingsKeepSourceViewsVisible(t *testing.T) {
	path := filepath.Join("..", "..", ".vscode", "settings.json")
	var document map[string]json.RawMessage
	readJSONC(t, path, &document)

	var associations map[string]string
	decodeSetting(t, document, "files.associations", &associations)
	if got := associations["*.kyse.go"]; got != "kyse" {
		t.Errorf("files.associations[*.kyse.go] = %q, want %q", got, "kyse")
	}

	var kyse struct {
		FormatOnSave *bool `json:"editor.formatOnSave"`
	}
	decodeSetting(t, document, "[kyse]", &kyse)
	if kyse.FormatOnSave == nil || *kyse.FormatOnSave {
		t.Error("Kyse must disable editor.formatOnSave")
	}
	if _, global := document["editor.formatOnSave"]; global {
		t.Error("editor.formatOnSave must not be disabled globally")
	}
	for key, raw := range document {
		if key == "[kyse]" || !strings.HasPrefix(key, "[") {
			continue
		}
		var language struct {
			FormatOnSave *bool `json:"editor.formatOnSave"`
		}
		if err := json.Unmarshal(raw, &language); err != nil {
			t.Fatalf("parse %s: %v", key, err)
		}
		if language.FormatOnSave != nil && !*language.FormatOnSave {
			t.Errorf("%s also disables editor.formatOnSave", key)
		}
	}

	var searchExcludes map[string]bool
	decodeSetting(t, document, "search.exclude", &searchExcludes)
	if len(searchExcludes) != 1 || !searchExcludes["storage/framework/views/**"] {
		t.Errorf("search.exclude = %#v, want only generated views", searchExcludes)
	}

	var fileExcludes map[string]bool
	decodeSetting(t, document, "files.exclude", &fileExcludes)
	for pattern, excluded := range fileExcludes {
		if excluded && strings.Contains(filepath.ToSlash(pattern), "resources/views") {
			t.Errorf("files.exclude hides source views with %q", pattern)
		}
	}
}

func readJSONC(t *testing.T, path string, destination any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(withoutJSONComments(contents), destination); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func decodeSetting(t *testing.T, document map[string]json.RawMessage, key string, destination any) {
	t.Helper()
	raw, ok := document[key]
	if !ok {
		t.Fatalf("editor setting %q is missing", key)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		t.Fatalf("parse editor setting %q: %v", key, err)
	}
}

func withoutJSONComments(contents []byte) []byte {
	clean := make([]byte, 0, len(contents))
	inString := false
	escaped := false
	for index := 0; index < len(contents); index++ {
		current := contents[index]
		if inString {
			clean = append(clean, current)
			switch {
			case escaped:
				escaped = false
			case current == '\\':
				escaped = true
			case current == '"':
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			clean = append(clean, current)
			continue
		}
		if current == '/' && index+1 < len(contents) && contents[index+1] == '/' {
			for index < len(contents) && contents[index] != '\n' {
				index++
			}
			if index < len(contents) {
				clean = append(clean, '\n')
			}
			continue
		}
		clean = append(clean, current)
	}
	return clean
}
