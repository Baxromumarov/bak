package cli

import (
	"fmt"
	"sort"
)

// Registry stores named command implementations and dispatches execution.
type Registry struct {
	commands map[string]Command
}

func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]Command)}
}

func (r *Registry) Register(cmd Command) {
	if cmd == nil {
		return
	}
	r.commands[cmd.Name()] = cmd
}

func (r *Registry) Has(name string) bool {
	_, ok := r.commands[name]
	return ok
}

func (r *Registry) Execute(ctx *Context, args []string) error {
	if len(args) == 0 {
		if cmd, ok := r.commands["repl"]; ok {
			return cmd.Run(ctx, nil)
		}
		return fmt.Errorf("missing command")
	}
	cmd, ok := r.commands[args[0]]
	if !ok {
		return fmt.Errorf("unknown command: %s", args[0])
	}
	return cmd.Run(ctx, args[1:])
}

func (r *Registry) CommandNames() []string {
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
