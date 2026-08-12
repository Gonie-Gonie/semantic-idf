package idf

import (
	"fmt"
	"strings"
)

func Parse(text string) (Document, error) {
	var doc Document
	var current Object
	var token strings.Builder
	inObject := false

	lines := strings.Split(text, "\n")
	for lineIndex, rawLine := range lines {
		code, comment, _ := strings.Cut(strings.TrimRight(rawLine, "\r"), "!")
		delimiterCount := countDelimiters(code)
		delimiterSeen := 0

		for _, r := range code {
			if r != ',' && r != ';' {
				token.WriteRune(r)
				continue
			}

			delimiterSeen++
			value := strings.TrimSpace(token.String())
			token.Reset()

			fieldComment := ""
			if delimiterSeen == delimiterCount {
				fieldComment = normalizeComment(comment)
			}

			if !inObject {
				if value == "" {
					continue
				}
				current = Object{Type: value}
				inObject = true
			} else {
				current.Fields = append(current.Fields, Field{
					Value:   value,
					Comment: fieldComment,
				})
			}

			if r == ';' {
				if !inObject {
					return doc, fmt.Errorf("line %d: object terminator without object", lineIndex+1)
				}
				current.Index = len(doc.Objects)
				doc.Objects = append(doc.Objects, current)
				current = Object{}
				inObject = false
			}
		}

		if delimiterCount == 0 && strings.TrimSpace(code) != "" {
			token.WriteByte(' ')
		}
	}

	if inObject {
		return doc, fmt.Errorf("unterminated object %q", current.Type)
	}
	if strings.TrimSpace(token.String()) != "" {
		return doc, fmt.Errorf("trailing content outside an object: %q", strings.TrimSpace(token.String()))
	}
	return doc, nil
}

func countDelimiters(value string) int {
	count := 0
	for _, r := range value {
		if r == ',' || r == ';' {
			count++
		}
	}
	return count
}

func normalizeComment(comment string) string {
	comment = strings.TrimSpace(comment)
	comment = strings.TrimPrefix(comment, "-")
	return strings.TrimSpace(comment)
}
