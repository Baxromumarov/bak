package typechecker

// ownershipBranch captures ownership facts produced in an isolated control-flow
// arm. A move is a "may move" fact: if any path reaching the join moved a
// value, using that value after the join is unsafe.
type ownershipBranch struct {
	moved    map[string]*MoveInfo
	poisoned map[string]bool
	parts    map[string]map[string]*MoveInfo
}

func captureOwnershipBranch(env *TypeEnv, parent *TypeEnv) ownershipBranch {
	branch := ownershipBranch{
		moved:    make(map[string]*MoveInfo),
		poisoned: make(map[string]bool),
		parts:    make(map[string]map[string]*MoveInfo),
	}
	for name, moved := range env.moved {
		if !moved {
			continue
		}
		if _, visible := parent.LookupSymbol(name); visible {
			branch.moved[name] = env.moveInfo[name]
		}
	}
	for name, poisoned := range env.poisoned {
		if poisoned {
			branch.poisoned[name] = true
		}
	}
	for root, paths := range env.movedParts {
		if _, visible := parent.LookupSymbol(root); !visible {
			continue
		}
		branch.parts[root] = make(map[string]*MoveInfo, len(paths))
		for path, info := range paths {
			branch.parts[root][path] = info
		}
	}
	return branch
}

func mergeOwnershipBranches(env *TypeEnv, branches ...ownershipBranch) {
	for _, branch := range branches {
		for name, info := range branch.moved {
			env.MarkMovedWithInfo(name, info)
		}
		for name := range branch.poisoned {
			env.MarkPoisoned(name)
		}
		for root, paths := range branch.parts {
			for path, info := range paths {
				env.MarkPartMoved(root, path, info)
			}
		}
	}
}
