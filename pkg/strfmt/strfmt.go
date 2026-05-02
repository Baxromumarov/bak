// Package strfmt provides ergonomic string-formatting helpers.
package strfmt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// builderPool reduces allocations for frequent small string builds.
var builderPool = sync.Pool{
	New: func() any { return new(strings.Builder) },
}

// =============================================================================
// Simple Concatenation
// =============================================================================

// S concatenates arguments into a string. It's faster than fmt.Sprint
// because it avoids unnecessary reflection where built-ins are used.
func S(parts ...any) string {
	b := builderPool.Get().(*strings.Builder)
	b.Reset()
	defer builderPool.Put(b)

	for _, p := range parts {
		writeAny(b, p)
	}
	return b.String()
}

// =============================================================================
// Named Placeholders
// =============================================================================

// Named replaces "{key}" in pattern using variadic key-value pairs.
func Named(pattern string, kv ...any) string {
	if len(kv) == 0 {
		return pattern
	}

	lookup := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		if key, ok := kv[i].(string); ok {
			lookup[key] = kv[i+1]
			lookup[strings.ToLower(key)] = kv[i+1]
		}
	}
	return formatNamed(pattern, lookup)
}

// Format replaces "{key}" using exported fields or JSON tags of a struct.
func Format(pattern string, v any) string {
	if v == nil {
		return pattern
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return pattern
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		// Fallback for non-structs: just try to stringify the whole thing?
		// Or return pattern. Here we stick to your original logic.
		return pattern
	}

	lookup := structToMap(rv)
	return formatNamed(pattern, lookup)
}

// =============================================================================
// Internal Helpers
// =============================================================================

func formatNamed(pattern string, lookup map[string]any) string {
	if len(lookup) == 0 {
		return pattern
	}

	b := builderPool.Get().(*strings.Builder)
	b.Reset()
	defer builderPool.Put(b)

	for i := 0; i < len(pattern); i++ {
		char := pattern[i]

		switch char {
		case '{':
			if i+1 < len(pattern) && pattern[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}

			// Find closing brace
			end := strings.IndexByte(pattern[i:], '}')
			if end == -1 {
				b.WriteByte('{')
				continue
			}

			absEnd := i + end
			key := pattern[i+1 : absEnd]

			if val, ok := lookup[key]; ok {
				writeAny(b, val)
			} else if val, ok := lookup[strings.ToLower(key)]; ok {
				writeAny(b, val)
			} else {
				b.WriteString(pattern[i : absEnd+1])
			}
			i = absEnd
		case '}':
			if i+1 < len(pattern) && pattern[i+1] == '}' {
				b.WriteByte('}')
				i++
			} else {
				b.WriteByte('}')
			}
		default:
			b.WriteByte(char)
		}
	}
	return b.String()
}

// structToMap extracts fields. In a production env, you'd cache the
// reflect.Type's field offsets to avoid repeating this loop.
func structToMap(rv reflect.Value) map[string]any {
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		val := rv.Field(i).Interface()
		out[field.Name] = val
		out[strings.ToLower(field.Name)] = val

		if tag := field.Tag.Get("json"); tag != "" && tag != "-" {
			name := strings.Split(tag, ",")[0]
			if name != "" {
				out[name] = val
			}
		}
	}
	return out
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
