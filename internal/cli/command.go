package cli

// Command defines a single CLI command implementation.
type Command interface {
	Name() string
	Description() string
	Run(ctx *Context, args []string) error
}
