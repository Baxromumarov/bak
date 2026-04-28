// Package strfmt provides ergonomic string-formatting helpers as a readable
// alternative to fmt.Sprintf for common concatenation and templating tasks.
//
// Simple left-to-right concatenation (no format verbs):
//
//	strfmt.S(startBracket, r.Start, ", ", r.End, endBracket)
//	// => "[3, 7]"
//
// Named placeholders with key-value pairs (recommended for this codebase):
//
//	strfmt.Named("User {name} has {points} points", "name", name, "points", points)
//
// Named placeholders with a struct (fields matched by name or json tag):
//
//	strfmt.Format("User {Name} has {Points} points", user)
//
// If you don't have a struct, use an inline struct:
//
//	strfmt.Format("User {Name} has {Points} points", struct {
//	    Name   string
//	    Points int
//	}{name, points})
//
// Fluent builder for complex assembly:
//
//	b := strfmt.NewBuilder()
//	b.Write("error: ", err).WriteString("\n")
//	return b.String()
package strfmt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// =============================================================================
// Simple concatenation
// =============================================================================

// S concatenates all arguments into a single string. It converts built-in types
// directly for efficiency; everything else falls back to fmt.Sprint.
//
// It is the recommended replacement for fmt.Sprintf when no special formatting
// (width, padding, base, precision, etc.) is required.
func S(parts ...any) string {
	var b strings.Builder
	for _, p := range parts {
		writeAny(&b, p)
	}
	return b.String()
}

// =============================================================================
// Named placeholders
// =============================================================================

// Named replaces each "{key}" in pattern using key-value pairs:
//
//	strfmt.Named("User {name} has {points} points", "name", name, "points", points)
//
// Keys are case-insensitive at lookup time ("{Name}" can match "name").
// If kv has an odd length, the last dangling key is ignored.
// Non-string keys are ignored.
func Named(pattern string, kv ...any) string {
	if len(kv) == 0 {
		return pattern
	}
	return formatNamed(pattern, lookupFromPairs(kv))
}

// Format replaces each occurrence of "{key}" in pattern with values from the
// exported fields of v. Keys are matched in this order:
//  1. Exact field name
//  2. JSON tag name (if present and not "-")
//  3. Case-insensitive field name
//
// Unmatched placeholders are left untouched. Literal braces can be written as
// "{{" and "}}".
func Format(pattern string, v any) string {
	if v == nil {
		return pattern
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return pattern
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return pattern
	}

	lookup := make(map[string]any)
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)
		if !field.IsExported() {
			continue
		}

		// Exact field name
		lookup[field.Name] = fv.Interface()

		// JSON tag
		if tag := field.Tag.Get("json"); tag != "" && tag != "-" {
			if idx := strings.Index(tag, ","); idx != -1 {
				tag = tag[:idx]
			}
			if tag != "" && tag != "-" {
				lookup[tag] = fv.Interface()
			}
		}

		// Lower-case variant
		lookup[strings.ToLower(field.Name)] = fv.Interface()
	}

	return formatNamed(pattern, lookup)
}

func lookupFromPairs(kv []any) map[string]any {
	lookup := make(map[string]any, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok || key == "" {
			continue
		}
		lookup[key] = kv[i+1]
		lookup[strings.ToLower(key)] = kv[i+1]
	}
	return lookup
}

// formatNamed is the shared implementation for all named formatting modes.
func formatNamed(pattern string, lookup map[string]any) string {
	if len(lookup) == 0 {
		return pattern
	}

	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '{' && i+1 < len(pattern) {
			// Escaped {{
			if pattern[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}
			// Find closing }
			j := i + 1
			for j < len(pattern) && pattern[j] != '}' {
				j++
			}
			if j < len(pattern) && pattern[j] == '}' {
				key := pattern[i+1 : j]
				if val, ok := lookup[key]; ok {
					writeAny(&b, val)
				} else if val, ok := lookup[strings.ToLower(key)]; ok {
					writeAny(&b, val)
				} else {
					b.WriteString(pattern[i : j+1])
				}
				i = j
				continue
			}
		} else if pattern[i] == '}' && i+1 < len(pattern) && pattern[i+1] == '}' {
			b.WriteByte('}')
			i++
			continue
		}
		b.WriteByte(pattern[i])
	}
	return b.String()
}

// =============================================================================
// Writer variants (drop-in replacements for fmt.Fprint / fmt.Fprintf)
// =============================================================================

// Fprint writes the concatenated parts to w. It mirrors fmt.Fprint but uses
// strfmt.S for formatting.
func Fprint(w io.Writer, parts ...any) (int, error) {
	return io.WriteString(w, S(parts...))
}

// Fprintln writes the concatenated parts to w followed by a newline.
func Fprintln(w io.Writer, parts ...any) (int, error) {
	return io.WriteString(w, S(parts...)+"\n")
}

// FprintFormat writes the struct-based template result to w.
func FprintFormat(w io.Writer, pattern string, v any) (int, error) {
	return io.WriteString(w, Format(pattern, v))
}

// =============================================================================
// Stdout variants (drop-in replacements for fmt.Print / fmt.Println)
// =============================================================================

// Print writes the concatenated parts to os.Stdout.
func Print(parts ...any) {
	_, _ = Fprint(os.Stdout, parts...)
}

// Println writes the concatenated parts to os.Stdout followed by a newline.
func Println(parts ...any) {
	_, _ = Fprintln(os.Stdout, parts...)
}

// PrintFormat writes the struct-based template result to os.Stdout.
func PrintFormat(pattern string, v any) {
	_, _ = FprintFormat(os.Stdout, pattern, v)
}

// =============================================================================
// Error variants (drop-in replacements for fmt.Errorf)
// =============================================================================

// Errorf returns an error built from concatenated parts.
func Errorf(parts ...any) error {
	return errors.New(S(parts...))
}

// ErrorfFormat returns an error built from a struct-based template.
func ErrorfFormat(pattern string, v any) error {
	return errors.New(Format(pattern, v))
}

// =============================================================================
// Error wrapping variants (replacement for fmt.Errorf("...: %w", err))
// =============================================================================

// wrappedError implements error wrapping compatible with errors.Is/As.
type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string { return e.msg + ": " + e.err.Error() }
func (e *wrappedError) Unwrap() error { return e.err }

// Wrap returns an error that wraps err with a message built from concatenated parts.
// If err is nil, Wrap returns nil.
func Wrap(err error, parts ...any) error {
	if err == nil {
		return nil
	}
	return &wrappedError{msg: S(parts...), err: err}
}

// WrapFormat wraps err with a struct-based template message.
// If err is nil, WrapFormat returns nil.
func WrapFormat(err error, pattern string, v any) error {
	if err == nil {
		return nil
	}
	return &wrappedError{msg: Format(pattern, v), err: err}
}

// =============================================================================
// Builder
// =============================================================================

// Builder wraps strings.Builder with helpers for fluent concatenation.
type Builder struct {
	b strings.Builder
}

// NewBuilder creates a new Builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// Write appends the arguments to the builder using the same conversion rules as S.
func (bb *Builder) Write(parts ...any) *Builder {
	for _, p := range parts {
		writeAny(&bb.b, p)
	}
	return bb
}

// WriteString appends a plain string to the builder.
func (bb *Builder) WriteString(s string) *Builder {
	bb.b.WriteString(s)
	return bb
}

// String returns the accumulated string.
func (bb *Builder) String() string {
	return bb.b.String()
}

// =============================================================================
// Internal helpers
// =============================================================================

// writeAny appends v to b using the most efficient conversion available.
func writeAny(b *strings.Builder, v any) {
	switch x := v.(type) {
	case string:
		b.WriteString(x)
	case []byte:
		b.Write(x)
	case int:
		b.WriteString(strconv.Itoa(x))
	case int8:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int16:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int32:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case uint:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint8:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint16:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint32:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint64:
		b.WriteString(strconv.FormatUint(x, 10))
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case float32:
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	case float64:
		b.WriteString(strconv.FormatFloat(x, 'g', -1, 64))
	case error:
		b.WriteString(x.Error())
	case fmt.Stringer:
		b.WriteString(x.String())
	default:
		fmt.Fprint(b, x)
	}
}
