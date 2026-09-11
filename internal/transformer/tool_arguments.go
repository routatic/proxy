package transformer

import (
	"encoding/json"
	"strings"
)

// normalizeToolArguments repairs whitespace around JSON object keys in
// provider-generated tool arguments. Some providers occasionally emit keys
// such as "description " instead of "description", which makes Claude
// Code reject an otherwise valid tool call during schema validation.
//
// Only key tokens are rewritten. Values, including numeric literals and
// string escapes, retain their original bytes. Decoding and re-encoding values
// can round numbers or replace unpaired Unicode surrogates.
//
// Invalid JSON or colliding keys leave the entire argument unchanged, so a
// repair never silently drops a value or hides an ambiguous tool call.
func normalizeToolArguments(raw string) string {
	if !json.Valid([]byte(raw)) {
		return raw
	}

	var out strings.Builder
	var objectKeys []map[string]struct{}
	copied := 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '{':
			objectKeys = append(objectKeys, make(map[string]struct{}))
		case '}':
			objectKeys = objectKeys[:len(objectKeys)-1]
		case '"':
			start := i
			// JSON validation above guarantees every string has a closing
			// quote and every backslash has a following escape character.
			i++
			for raw[i] != '"' {
				if raw[i] == '\\' {
					i++
				}
				i++
			}
			end := i + 1
			next := end
			for next < len(raw) && strings.ContainsRune(" \t\r\n", rune(raw[next])) {
				next++
			}
			// In valid JSON, only object keys are followed by a colon.
			// Skip string values without decoding their contents.
			if next == len(raw) || raw[next] != ':' {
				continue
			}

			var key string
			if err := json.Unmarshal([]byte(raw[start:end]), &key); err != nil {
				return raw
			}
			trimmed := strings.TrimSpace(key)
			keys := objectKeys[len(objectKeys)-1]
			if _, exists := keys[trimmed]; exists {
				return raw
			}
			keys[trimmed] = struct{}{}
			if trimmed == key {
				continue
			}
			// The decoder substitutes U+FFFD for invalid Unicode. Decline
			// repair when a key may have lost information during decoding.
			if strings.ContainsRune(key, '\uFFFD') {
				return raw
			}
			if copied == 0 {
				out.Grow(len(raw))
			}
			out.WriteString(raw[copied:start])
			encoded, _ := json.Marshal(trimmed) // Strings always marshal.
			out.Write(encoded)
			copied = end
		}
	}
	if copied == 0 {
		return raw
	}
	out.WriteString(raw[copied:])
	return out.String()
}
