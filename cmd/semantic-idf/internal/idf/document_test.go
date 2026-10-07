package idf

import "testing"

func TestDocumentStringPreservesCommentAlignment(t *testing.T) {
	doc := Document{Objects: []Object{
		{Type: "Empty"},
		{Type: "Thing", Fields: []Field{
			{Value: "value", Comment: "Label"},
			{Value: "12345678901234567890123456789", Comment: "At boundary"},
			{Value: "123456789012345678901234567890", Comment: "Long"},
		}},
		{Type: "Raw", Fields: []Field{{Value: "100%"}, {Value: ""}}},
	}}
	const want = `Empty;

Thing,
  value,                        !- Label
  12345678901234567890123456789,  !- At boundary
  123456789012345678901234567890;  !- Long

Raw,
  100%,
  ;
`
	if got := doc.String(); got != want {
		t.Fatalf("IDF serialization changed:\ngot  %q\nwant %q", got, want)
	}
	if got := (Document{}).String(); got != "" {
		t.Fatalf("empty document serialization = %q, want empty", got)
	}
}
