package widget

import (
	"encoding/json"
	"strings"
	"testing"
)

func widgetArgs(t *testing.T, js string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestShowWidgetRefusesInputOutsideTheSchemaAndLimits(t *testing.T) {
	rows := make([]string, MaxRows+1)
	for i := range rows {
		rows[i] = `{"cells":["x"]}`
	}
	cases := map[string]string{
		"unknown kind":           `{"kind":"iframe"}`,
		"missing block":          `{"kind":"table"}`,
		"bad id":                 `{"id":"Deploy!","kind":"keyvalue","keyvalue":{"items":[{"key":"a","value":"b"}]}}`,
		"hex tone":               `{"kind":"keyvalue","keyvalue":{"items":[{"key":"a","value":"b","tone":"#ff0000"}]}}`,
		"too many rows":          `{"kind":"table","table":{"columns":[{"title":"a"}],"rows":[` + strings.Join(rows, ",") + `]}}`,
		"more cells than cols":   `{"kind":"table","table":{"columns":[{"title":"a"}],"rows":[{"cells":["1","2"]}]}}`,
		"progress over one":      `{"kind":"progress","progress":{"bars":[{"label":"a","value":1.5}]}}`,
		"seven buttons":          `{"kind":"buttons","buttons":{"buttons":[{"label":"1"},{"label":"2"},{"label":"3"},{"label":"4"},{"label":"5"},{"label":"6"},{"label":"7"}]}}`,
		"duplicate buttons":      `{"kind":"buttons","buttons":{"buttons":[{"label":"Go"},{"label":"Go"}]}}`,
		"choice default missing": `{"kind":"form","form":{"fields":[{"name":"env","label":"Env","kind":"choice","options":["dev"],"default":"prod"}]}}`,
		"checkbox field":         `{"kind":"form","form":{"fields":[{"name":"ok","label":"OK","kind":"checkbox"}]}}`,
		"series lengths differ":  `{"kind":"chart","chart":{"type":"line","series":[{"values":[1,2]},{"values":[1]}]}}`,
		"three series":           `{"kind":"chart","chart":{"type":"bar","series":[{"values":[1]},{"values":[1]},{"values":[1]}]}}`,
	}
	for name, js := range cases {
		if _, err := Parse(widgetArgs(t, js)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	big := `{"kind":"keyvalue","keyvalue":{"items":[{"key":"a","value":"` + strings.Repeat("x", maxBytes) + `"}]}}`
	if _, err := Parse(widgetArgs(t, big)); err == nil || !strings.Contains(err.Error(), "64 KB") {
		t.Errorf("a widget over 64 KB gave %v", err)
	}
	w, err := Parse(widgetArgs(t, `{"kind":"table","table":{"columns":[{"title":"a"},{"title":"b"}],"rows":[{"cells":["`+strings.Repeat("y", 300)+`"]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c := w.Table.Rows[0].Cells; len(c) != 2 || len([]rune(c[0])) != maxCell {
		t.Errorf("a long short row became %d cells, the first %d characters", len(c), len([]rune(c[0])))
	}
}
