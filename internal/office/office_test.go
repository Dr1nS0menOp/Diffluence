// SPDX-License-Identifier: Apache-2.0

package office

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
)

func parse(t *testing.T, doc string) document.Node {
	t.Helper()
	var n document.Node
	if err := json.Unmarshal([]byte(doc), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

const sample = `{"type":"doc","content":[
	{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"First <part>"}]},
	{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"bold"}]},
		{"type":"text","text":" site","marks":[{"type":"link","attrs":{"href":"https://example.com/a"}}]},
		{"type":"text","text":" hidden","marks":[{"type":"link","attrs":{"href":"/pages/secret"}}]}]},
	{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"item"}]}]}]},
	{"type":"table","content":[
		{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"Name"}]}]}]},
		{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"Anna"}]}]}]}]},
	{"type":"image","attrs":{"attachment_id":"IMG","alt":"A map"}},
	{"type":"panel","attrs":{"kind":"warning"},"content":[{"type":"paragraph","content":[{"type":"text","text":"Mind this"}]}]}
]}`

func options(id uuid.UUID) Options {
	return Options{Image: func(got uuid.UUID) (Image, bool) { return Image{Data: []byte("png"), Ext: "png"}, got == id }}
}

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func wellFormed(t *testing.T, name, body string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("%s is not well-formed: %v", name, err)
		}
	}
}

func TestDOCXHasRealHeadingsTableHeadersAndAltText(t *testing.T) {
	id := uuid.New()
	doc := parse(t, strings.ReplaceAll(sample, "IMG", id.String()))
	var b bytes.Buffer
	if err := WriteDOCX(&b, []Section{{Title: "Page", Document: doc}}, options(id)); err != nil {
		t.Fatal(err)
	}
	files := unzip(t, b.Bytes())
	for name, body := range files {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			wellFormed(t, name, body)
		}
	}
	main := files["word/document.xml"]
	for _, want := range []string{`<w:pStyle w:val="Heading1"/>`, `<w:pStyle w:val="Heading3"/>`, "First &lt;part&gt;", "<w:tblHeader/>",
		`descr="A map"`, "<w:numId", ">Warning<"} {
		if !strings.Contains(main, want) {
			t.Errorf("missing %s in %s", want, main)
		}
	}
	if _, ok := files["word/media/image1.png"]; !ok {
		t.Error("the image is not in the file")
	}
	if !strings.Contains(files["word/_rels/document.xml.rels"], "https://example.com/a") {
		t.Error("the link to the outside is gone")
	}
	if strings.Contains(main+files["word/_rels/document.xml.rels"], "/pages/secret") {
		t.Error("an address inside the instance left in the file")
	}
}

func TestODTIsAValidPackageWithItsMimetypeFirst(t *testing.T) {
	id := uuid.New()
	doc := parse(t, strings.ReplaceAll(sample, "IMG", id.String()))
	var b bytes.Buffer
	if err := WriteODT(&b, []Section{{Title: "Page", Document: doc}}, options(id)); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store {
		t.Fatalf("first entry: %s %d", zr.File[0].Name, zr.File[0].Method)
	}
	files := unzip(t, b.Bytes())
	wellFormed(t, "content.xml", files["content.xml"])
	wellFormed(t, "manifest", files["META-INF/manifest.xml"])
	content := files["content.xml"]
	for _, want := range []string{`text:outline-level="3"`, "<table:table-header-rows>", "<svg:desc>A map</svg:desc>", `xlink:href="https://example.com/a"`} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(content, "/pages/secret") {
		t.Error("an address inside the instance left in the file")
	}
}

func TestABranchMovesHeadingsDownALevelPerPage(t *testing.T) {
	doc := parse(t, `{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"x"}]}]}`)
	var b bytes.Buffer
	if err := WriteDOCX(&b, []Section{{Title: "Deep", Depth: 2, Document: doc}}, Options{}); err != nil {
		t.Fatal(err)
	}
	main := unzip(t, b.Bytes())["word/document.xml"]
	if !strings.Contains(main, `w:val="Heading3"`) || !strings.Contains(main, `w:val="Heading4"`) {
		t.Fatalf("got %s", main)
	}
}

func TestReadDOCXBringsBackWhatWeWrote(t *testing.T) {
	id := uuid.New()
	doc := parse(t, strings.ReplaceAll(sample, "IMG", id.String()))
	var b bytes.Buffer
	if err := WriteDOCX(&b, []Section{{Title: "Page", Document: doc}}, options(id)); err != nil {
		t.Fatal(err)
	}
	stored := 0
	got, err := ReadDOCX(b.Bytes(), func(name string, data []byte) (map[string]any, bool) {
		stored++
		return map[string]any{"attachment_id": id.String()}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	document.Walk(got.Document, func(n document.Node) { types[n.Type]++ })
	if types[document.TypeHeading] < 2 || types[document.TypeBulletList] != 1 || types[document.TypeTable] < 1 ||
		types[document.TypeTableHeader] != 1 || types[document.TypeImage] != 1 || stored != 1 {
		t.Fatalf("types %v, stored %d", types, stored)
	}
	raw, _ := json.Marshal(got.Document)
	if _, err := document.Normalize(raw); err != nil {
		t.Fatalf("the allow-list refuses what we read: %v\n%s", err, raw)
	}
	if !strings.Contains(string(raw), `"href":"https://example.com/a"`) {
		t.Errorf("the link is gone: %s", raw)
	}
}

func hostile(t *testing.T, document, rels string, extra map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	files := map[string]string{
		"_rels/.rels":                  `<Relationships><Relationship Id="r" Type="x/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":            document,
		"word/_rels/document.xml.rels": rels,
	}
	for k, v := range extra {
		files[k] = v
	}
	for name, body := range files {
		f, _ := zw.Create(name)
		_, _ = io.WriteString(f, body)
	}
	_ = zw.Close()
	return b.Bytes()
}

func TestReadDOCXNeverFollowsAnExternalImage(t *testing.T) {
	doc := `<w:document><w:body><w:p><w:r><w:drawing><a:blip r:embed="r1"/></w:drawing></w:r></w:p></w:body></w:document>`
	rels := `<Relationships><Relationship Id="r1" Target="http://evil.example.com/x.png" TargetMode="External"/></Relationships>`
	got, err := ReadDOCX(hostile(t, doc, rels, nil), func(string, []byte) (map[string]any, bool) {
		t.Fatal("an external image reached the store")
		return nil, false
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("warnings: %v", got.Warnings)
	}
}

func TestReadDOCXDoesNotStepOutOfThePackage(t *testing.T) {
	doc := `<w:document><w:body><w:p><w:r><w:drawing><a:blip r:embed="r1"/></w:drawing></w:r></w:p></w:body></w:document>`
	rels := `<Relationships><Relationship Id="r1" Target="../../../etc/passwd"/></Relationships>`
	if _, err := ReadDOCX(hostile(t, doc, rels, nil), func(string, []byte) (map[string]any, bool) {
		t.Fatal("a path outside the package was read")
		return nil, false
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReadDOCXDropsMacrosAndKeepsSayingSo(t *testing.T) {
	doc := `<w:document><w:body><w:p><w:del><w:r><w:t>gone</w:t></w:r></w:del><w:ins><w:r><w:t>kept</w:t></w:r></w:ins></w:p></w:body></w:document>`
	got, err := ReadDOCX(hostile(t, doc, "<Relationships/>", map[string]string{"word/vbaProject.bin": "x"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	text := document.Text(got.Document)
	if strings.Contains(text, "gone") || !strings.Contains(text, "kept") {
		t.Fatalf("text %q", text)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "Macros") {
		t.Fatalf("warnings %v", got.Warnings)
	}
}

func TestReadDOCXRefusesWhatIsNotAWordFile(t *testing.T) {
	for name, data := range map[string][]byte{
		"old word":  {0xD0, 0xCF, 0x11, 0xE0, 1, 2},
		"not a zip": []byte("hello"),
		"no doc":    hostileEmpty(t),
		"deep":      hostile(t, strings.Repeat("<a>", maxXMLDepth+5), "<Relationships/>", nil),
	} {
		if _, err := ReadDOCX(data, nil); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func hostileEmpty(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	f, _ := zw.Create("readme.txt")
	_, _ = io.WriteString(f, "x")
	_ = zw.Close()
	return b.Bytes()
}

func TestAnEntityInAWordFileExpandsToNothing(t *testing.T) {
	doc := `<!DOCTYPE d [<!ENTITY x "boom"><!ENTITY f SYSTEM "file:///etc/passwd">]><w:document><w:body><w:p><w:r><w:t>a&x;&f;</w:t></w:r></w:p></w:body></w:document>`
	got, err := ReadDOCX(hostile(t, doc, "<Relationships/>", nil), nil)
	if err != nil {
		return // refusing is fine too
	}
	if text := document.Text(got.Document); strings.Contains(text, "boom") || strings.Contains(text, "root:") {
		t.Fatalf("an entity was expanded: %q", text)
	}
}
