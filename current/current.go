// Package current contains context overridable globals.
//
// See https://www.jackchristensen.com/2020/04/11/the-context-overridable-global-pattern-in-go.html
package current

type ctxKey int

const (
	_ ctxKey = iota
	secretKeyBaseCtxKey
)
