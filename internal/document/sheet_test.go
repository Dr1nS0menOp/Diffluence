// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func sheetDoc(columns, rows string) []byte {
	return fmt.Appendf(nil, `{"type":"doc","content":[{"type":"sheet","attrs":{"columns":%s,"rows":%s}}]}`, columns, rows)
}

const twoColumns = `[{"id":"a","name":"Name","type":"text"},{"id":"b","name":"Site","type":"link"}]`

func TestASheetThatBreaksARuleIsRefused(t *testing.T) {
	cases := map[string][]byte{
		"no columns":        sheetDoc(`[]`, `[]`),
		"unknown type":      sheetDoc(`[{"id":"a","name":"N","type":"script"}]`, `[]`),
		"double column":     sheetDoc(`[{"id":"a","name":"N","type":"text"},{"id":"a","name":"M","type":"text"}]`, `[]`),
		"bad column id":     sheetDoc(`[{"id":"a b","name":"N","type":"text"}]`, `[]`),
		"empty name":        sheetDoc(`[{"id":"a","name":" ","type":"text"}]`, `[]`),
		"name on two lines": sheetDoc(`[{"id":"a","name":"N\nM","type":"text"}]`, `[]`),
		"double row":        sheetDoc(twoColumns, `[{"id":"r","cells":{}},{"id":"r","cells":{}}]`),
		"row without id":    sheetDoc(twoColumns, `[{"cells":{}}]`),
		"cell not text":     sheetDoc(twoColumns, `[{"id":"r","cells":{"a":5}}]`),
		"script link":       sheetDoc(twoColumns, `[{"id":"r","cells":{"b":"javascript:alert(1)"}}]`),
		"formula as link":   sheetDoc(twoColumns, `[{"id":"r","cells":{"b":"=A1"}}]`),
		"bad date":          sheetDoc(`[{"id":"a","name":"N","type":"date"}]`, `[{"id":"r","cells":{"a":"6 November"}}]`),
		"bad tick":          sheetDoc(`[{"id":"a","name":"N","type":"check"}]`, `[{"id":"r","cells":{"a":"yes"}}]`),
		"long cell":         sheetDoc(twoColumns, `[{"id":"r","cells":{"a":"`+strings.Repeat("x", maxSheetCell+1)+`"}}]`),
		"inside a panel":    []byte(`{"type":"doc","content":[{"type":"panel","attrs":{"kind":"info"},"content":[{"type":"sheet","attrs":{"columns":` + twoColumns + `,"rows":[]}}]}]}`),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Normalize(raw); err == nil {
				t.Fatal("it passed the gate")
			}
		})
	}
}

func TestASheetHasLimits(t *testing.T) {
	columns := make([]string, maxSheetColumns+1)
	for i := range columns {
		columns[i] = fmt.Sprintf(`{"id":"c%d","name":"N","type":"text"}`, i)
	}
	if _, err := Normalize(sheetDoc("["+strings.Join(columns, ",")+"]", `[]`)); err == nil {
		t.Fatal("too many columns")
	}
	rows := make([]string, maxSheetRows+1)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"id":"r%d","cells":{}}`, i)
	}
	if _, err := Normalize(sheetDoc(twoColumns, "["+strings.Join(rows, ",")+"]")); err == nil {
		t.Fatal("too many rows")
	}
}

func TestACleanSheetKeepsItsDataAndDropsTheRest(t *testing.T) {
	raw := sheetDoc(
		`[{"id":"a","name":"Name","type":"text","onclick":"x"},{"id":"c","name":"State","type":"choice","options":["Open","Done"]}]`,
		`[{"id":"r1","extra":"x","cells":{"a":"=SUM(B1:B2)","c":"Open","unknown":"gone"}},{"id":"r2","cells":{"a":""}}]`)
	doc, err := Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"onclick", "unknown", "extra"} {
		if strings.Contains(string(stored), gone) {
			t.Errorf("%s came through: %s", gone, stored)
		}
	}
	for _, kept := range []string{"=SUM(B1:B2)", "Open", "Done"} {
		if !strings.Contains(string(stored), kept) {
			t.Errorf("%s is lost: %s", kept, stored)
		}
	}
}

func TestAnEmptySheetPassesTheGate(t *testing.T) {
	raw, err := json.Marshal(Node{Type: TypeDoc, Content: EmptySheet("Name", "Notes").Content})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !IsSheetDocument(doc) || !HasSheet(doc) {
		t.Fatalf("not a sheet: %+v", doc)
	}
}

func TestOnlyOneSheetAloneIsASheetDocument(t *testing.T) {
	sheet := Node{Type: TypeSheet}
	text := Node{Type: TypeParagraph}
	if IsSheetDocument(Node{Content: []Node{sheet, text}}) || IsSheetDocument(Node{Content: []Node{text}}) || IsSheetDocument(Node{}) {
		t.Fatal("that is not a database")
	}
}

func TestTheWordsOfASheetCanBeFoundAndExported(t *testing.T) {
	doc, err := Normalize(sheetDoc(twoColumns, `[{"id":"r","cells":{"a":"Kiwi|fruit","b":"https://example.com/x"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	if text := Text(doc); !strings.Contains(text, "Kiwi|fruit") || !strings.Contains(text, "Name") {
		t.Fatalf("search text: %q", text)
	}
	md := ToMarkdown(doc, nil)
	if !strings.Contains(md, `| Kiwi\|fruit |`) || !strings.Contains(md, "| --- | --- |") {
		t.Fatalf("markdown: %q", md)
	}
}

func TestASheetBecomesAnEscapedTableInHTML(t *testing.T) {
	doc, err := Normalize(sheetDoc(twoColumns, `[{"id":"r","cells":{"a":"<script>alert(1)</script>"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	out := ToHTML(doc, HTMLOptions{})
	if strings.Contains(out, "<script") || !strings.Contains(out, "<th>Name</th>") || !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("html: %s", out)
	}
}
