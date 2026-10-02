package request

import "strings"

var likeEscaper = strings.NewReplacer(`\`, `\`, `%`, `\%`, `_`, `\_`)

// EscapeLike escapes the LIKE / ILIKE wildcards in a user-supplied search term
// so it is matched literally (PostgreSQL's default escape character is `\`).
func EscapeLike(s string) string {
	return likeEscaper.Replace(s)
}
