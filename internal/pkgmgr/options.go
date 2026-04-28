package pkgmgr

import "fmt"

// Options configures package manager operations.
type Options struct {
	Offline        bool
	FrozenLockfile bool
}

func ParseOptions(args []string) (Options, []string, error) {
	var opts Options
	var rest []string
	for _, arg := range args {
		switch arg {
		case "--offline":
			opts.Offline = true
		case "--frozen-lockfile":
			opts.FrozenLockfile = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return Options{},
					nil,
					fmt.Errorf("unknown dependency flag: %s", arg)
			}
			rest = append(rest, arg)
		}
	}
	return opts, rest, nil
}
