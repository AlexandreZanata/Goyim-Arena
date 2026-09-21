package main

import (
	"fmt"
	"strings"
)

// parse turns a Caddyfile into statements.
//
// It is deliberately a small parser for a small language: a Caddyfile is a
// sequence of statements, a statement is the tokens on one line, and a
// statement may open a block whose contents are statements again. Nothing else
// is needed to answer the questions this audit asks, and a parser that
// understood more would be a parser whose extra understanding could disagree
// with Caddy's own.
//
// Two details of Caddy's lexer are load-bearing here and are therefore handled
// rather than approximated:
//
//   - a line comment starts at a `#` that begins a token, and runs to the end
//     of the line. A `#` inside a quoted string is content — which matters,
//     because a CSP value is exactly the kind of value that could contain one;
//   - `{` on its own opens a block, while a token that merely starts with `{`
//     is a placeholder: `{client_ip}` and `{$COMPOSE_SITE_ADDRESS}` are values,
//     not braces. A parser that mistook one for the other would read a site
//     block as an argument and then report the whole file as broken.
func parse(raw []byte) ([]statement, error) {
	tokens, err := lex(string(raw))
	if err != nil {
		return nil, err
	}
	cursor := 0
	statements, err := parseStatements(tokens, &cursor)
	if err != nil {
		return nil, err
	}
	if cursor != len(tokens) {
		return nil, fmt.Errorf("line %d: unexpected %q after the last block", tokens[cursor].line, tokens[cursor].text)
	}
	return statements, nil
}

// parseStatements reads statements until the end of the input or a closing
// brace, which it consumes.
func parseStatements(tokens []token, cursor *int) ([]statement, error) {
	statements := []statement{}
	for *cursor < len(tokens) {
		if tokens[*cursor].text == "}" {
			*cursor++
			return statements, nil
		}

		line := tokens[*cursor].line
		args := []string{}
		for *cursor < len(tokens) && tokens[*cursor].text != "{" && tokens[*cursor].text != "}" && tokens[*cursor].line == line {
			args = append(args, tokens[*cursor].text)
			*cursor++
		}

		current := statement{args: args, line: line}
		if *cursor < len(tokens) && tokens[*cursor].text == "{" {
			*cursor++
			children, err := parseStatements(tokens, cursor)
			if err != nil {
				return nil, err
			}
			current.children = children
		}
		statements = append(statements, current)
	}
	return statements, nil
}

// lex splits the file into tokens, dropping comments and quotes.
func lex(source string) ([]token, error) {
	tokens := []token{}
	line := 1
	index := 0

	for index < len(source) {
		character := source[index]

		switch {
		case character == '\n':
			line++
			index++
		case character == ' ' || character == '\t' || character == '\r':
			index++
		case character == '#' && atTokenStart(source, index):
			for index < len(source) && source[index] != '\n' {
				index++
			}
		case (character == '{' || character == '}') && standalone(source, index):
			// A brace is a block delimiter only when it stands alone. A token
			// that merely starts with one is a placeholder, and the two must not
			// be confused: `{$COMPOSE_SITE_ADDRESS:arena.invalid}` written as a
			// block would turn one site into an empty statement with a
			// body, and every rule below would then be reading the wrong tree.
			tokens = append(tokens, token{text: string(character), line: line})
			index++
		case character == '"' || character == '`':
			quote := character
			index++
			start := index
			escaped := false
			for index < len(source) {
				if source[index] == '\n' {
					line++
				}
				if escaped {
					escaped = false
					index++
					continue
				}
				if source[index] == '\\' && quote == '"' {
					escaped = true
					index++
					continue
				}
				if source[index] == quote {
					break
				}
				index++
			}
			if index >= len(source) {
				return nil, fmt.Errorf("line %d: a quoted value is never closed", line)
			}
			tokens = append(tokens, token{text: unescape(source[start:index]), line: line})
			index++
		default:
			start := index
			for index < len(source) && !isSeparator(source[index]) {
				index++
			}
			tokens = append(tokens, token{text: source[start:index], line: line})
		}
	}

	return tokens, nil
}

// isSeparator reports whether a byte ends a token.
func isSeparator(character byte) bool {
	return character == ' ' || character == '\t' || character == '\r' || character == '\n'
}

// standalone reports whether the brace at index stands alone, which is what
// makes it a block delimiter rather than the first byte of a placeholder.
func standalone(source string, index int) bool {
	following := index + 1
	return following >= len(source) || isSeparator(source[following])
}

// atTokenStart reports whether the byte at index begins a token, which is what
// makes a `#` a comment rather than part of one.
func atTokenStart(source string, index int) bool {
	if index == 0 {
		return true
	}
	return isSeparator(source[index-1])
}

// unescape removes the backslashes the lexer kept, so that a quoted value is
// compared as the value Caddy would see.
func unescape(value string) string {
	if !strings.Contains(value, "\\") {
		return value
	}
	var builder strings.Builder
	escaped := false
	for _, character := range value {
		switch {
		case escaped:
			builder.WriteRune(character)
			escaped = false
		case character == '\\':
			escaped = true
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}
