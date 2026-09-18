package domain

import (
	"strings"
	"unicode/utf8"
)

// ValidatePayload checks a job payload without parsing it.
//
// Domain code cannot depend on a serialization library (the architecture gate
// forbids encoding/json here), and it should not have to: the queue only needs
// to refuse what cannot be a bounded JSON object. PostgreSQL's jsonb is the
// authority that ultimately parses the document, so this check is a structural
// precondition — object-shaped, UTF-8, bounded size, bounded nesting and no
// control characters — that makes an unrepresentable payload impossible to
// enqueue instead of letting it fail inside a statement.
func ValidatePayload(payload []byte) error {
	if len(payload) == 0 {
		return ErrEmptyPayload
	}
	if len(payload) > MaxPayloadBytes {
		return ErrPayloadTooLarge
	}
	if !utf8.Valid(payload) {
		return ErrPayloadMalformed
	}
	text := string(payload)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return ErrPayloadNotObject
	}
	if err := checkObjectStructure(text); err != nil {
		return err
	}
	return nil
}

// checkObjectStructure walks the document once, tracking string literals and
// escape sequences, and rejects unbalanced or over-nested input.
func checkObjectStructure(text string) error {
	depth := 0
	inString := false
	escaped := false
	closed := false

	for index := 0; index < len(text); index++ {
		char := text[index]

		if inString {
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			case char < 0x20:
				// A raw control character is never valid inside a JSON
				// string; it would also let a payload carry line breaks
				// into logs and error details.
				return ErrPayloadMalformed
			}
			continue
		}

		switch char {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > MaxPayloadDepth {
				return ErrPayloadTooDeep
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return ErrPayloadMalformed
			}
			if depth == 0 {
				closed = true
			}
		case ' ', '\t', '\n', '\r', ',', ':':
			// Structure and separators are fine outside strings.
		default:
			if char < 0x20 {
				return ErrPayloadMalformed
			}
		}

		if closed && index != len(text)-1 {
			// Trailing content after the closing brace means the document
			// is not the single object the column expects.
			rest := strings.TrimSpace(text[index+1:])
			if rest != "" {
				return ErrPayloadNotObject
			}
		}
	}

	if inString || escaped || depth != 0 {
		return ErrPayloadMalformed
	}
	return nil
}
