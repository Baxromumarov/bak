package typechecker

import (
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
)

func sortedOwnershipPaths(paths map[string]*MoveInfo) []string {
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	return keys
}

// ownershipPathsOverlap reports whether two projected places may refer to the
// same storage. Constant indexes remain independent, while a dynamic index is
// treated as potentially aliasing every index at the same projection level.
func ownershipPathsOverlap(left, right string) bool {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	limit := min(len(leftParts), len(rightParts))

	for i := 0; i < limit; i++ {
		if leftParts[i] == rightParts[i] {
			continue
		}
		leftIndex := strings.HasPrefix(leftParts[i], "[") && strings.HasSuffix(leftParts[i], "]")
		rightIndex := strings.HasPrefix(rightParts[i], "[") && strings.HasSuffix(rightParts[i], "]")
		if leftIndex && rightIndex && (leftParts[i] == "[]" || rightParts[i] == "[]") {
			continue
		}
		return false
	}

	return true
}

// movedPathAffectsQuery is directional: a moved parent makes its children
// unavailable, but querying a parent while resolving a longer projection is
// not itself a use of the whole parent value.
func movedPathAffectsQuery(movedPath, queryPath string) bool {
	if !ownershipPathsOverlap(movedPath, queryPath) {
		return false
	}
	return len(strings.Split(movedPath, ".")) <= len(strings.Split(queryPath, "."))
}

func (e *TypeEnv) MarkPartMoved(root, path string, info *MoveInfo) {
	if _, owns := e.symbols[root]; owns || e.isolated {
		if e.movedParts[root] == nil {
			e.movedParts[root] = make(map[string]*MoveInfo)
		}
		e.movedParts[root][path] = info
		return
	}
	if e.parent != nil {
		e.parent.MarkPartMoved(root, path, info)
	}
}

func (e *TypeEnv) PartMoveInfo(root, path string) *MoveInfo {
	if paths := e.movedParts[root]; paths != nil {
		for _, movedPath := range sortedOwnershipPaths(paths) {
			if movedPathAffectsQuery(movedPath, path) {
				return paths[movedPath]
			}
		}
	}
	if e.parent != nil {
		return e.parent.PartMoveInfo(root, path)
	}
	return nil
}

// PartMoveInfoForConsume also detects moved children because consuming a
// parent place would consume/drop every child it owns.
func (e *TypeEnv) PartMoveInfoForConsume(root, path string) *MoveInfo {
	if paths := e.movedParts[root]; paths != nil {
		for _, movedPath := range sortedOwnershipPaths(paths) {
			if ownershipPathsOverlap(movedPath, path) {
				return paths[movedPath]
			}
		}
	}
	if e.parent != nil {
		return e.parent.PartMoveInfoForConsume(root, path)
	}
	return nil
}

func (e *TypeEnv) HasMovedPart(root string) bool {
	if len(e.movedParts[root]) > 0 {
		return true
	}
	return e.parent != nil && e.parent.HasMovedPart(root)
}

func (e *TypeEnv) ClearMovedPart(root, path string) {
	if paths := e.movedParts[root]; paths != nil {
		// A concrete parent assignment initializes all of its descendants. An
		// unknown dynamic index cannot prove which element was initialized.
		if !strings.Contains(path, "[]") {
			prefix := path + "."
			for movedPath := range paths {
				if movedPath == path || strings.HasPrefix(movedPath, prefix) {
					delete(paths, movedPath)
				}
			}
		}
		if len(paths) == 0 {
			delete(e.movedParts, root)
		}
		return
	}
	if e.parent != nil {
		e.parent.ClearMovedPart(root, path)
	}
}

// MarkMovedWithInfo marks a variable as moved with tracking info
func (e *TypeEnv) MarkMovedWithInfo(name string, info *MoveInfo) {
	// Find which environment owns this variable and mark it there
	if _, ok := e.symbols[name]; ok {
		e.moved[name] = true
		e.moveInfo[name] = info
		return
	}
	// If this environment is isolated, mark the move locally to shadow parent
	if e.isolated {
		e.moved[name] = true
		e.moveInfo[name] = info
		return
	}
	if e.parent != nil {
		e.parent.MarkMovedWithInfo(name, info)
	}
}

// MarkMoved marks a variable as moved (ownership transferred) - legacy support
func (e *TypeEnv) MarkMoved(name string) {
	e.MarkMovedWithInfo(name, nil)
}

// ClearMoved marks storage as initialized again after a successful assignment.
func (e *TypeEnv) ClearMoved(name string) {
	if e.moved[name] || e.isolated {
		delete(e.moved, name)
		delete(e.moveInfo, name)
		delete(e.movedParts, name)
		delete(e.poisoned, name)
		return
	}
	if _, ok := e.symbols[name]; ok {
		delete(e.moved, name)
		delete(e.moveInfo, name)
		delete(e.movedParts, name)
		delete(e.poisoned, name)
		return
	}
	if e.parent != nil {
		e.parent.ClearMoved(name)
	}
}

// GetMoveInfo returns the move info for a variable
func (e *TypeEnv) GetMoveInfo(name string) *MoveInfo {
	if _, ok := e.moved[name]; ok {
		return e.moveInfo[name]
	}
	if _, ok := e.symbols[name]; ok {
		return e.moveInfo[name]
	}
	if e.parent != nil {
		return e.parent.GetMoveInfo(name)
	}
	return nil
}

// IsMoved checks if a variable has been moved
func (e *TypeEnv) IsMoved(name string) bool {
	if e.moved[name] {
		return true
	}
	if _, ok := e.symbols[name]; ok {
		return e.moved[name]
	}
	if e.parent != nil {
		return e.parent.IsMoved(name)
	}
	return false
}

// MarkPoisoned marks a variable as poisoned (suppress further errors)
func (e *TypeEnv) MarkPoisoned(name string) {
	if e.isolated {
		e.poisoned[name] = true
		return
	}
	if _, ok := e.symbols[name]; ok {
		e.poisoned[name] = true
		return
	}
	if e.parent != nil {
		e.parent.MarkPoisoned(name)
	}
}

// IsPoisoned checks if a variable is poisoned
func (e *TypeEnv) IsPoisoned(name string) bool {
	if e.poisoned[name] {
		return true
	}
	if _, ok := e.symbols[name]; ok {
		return e.poisoned[name]
	}
	if e.parent != nil {
		return e.parent.IsPoisoned(name)
	}
	return false
}

// MarkBorrowedMut marks a variable as mutably borrowed in the CURRENT scope only.
func (e *TypeEnv) MarkBorrowedMut(name string) {
	e.MarkBorrowedMutAt(name, ast.Position{})
}

// MarkBorrowedMutAt marks a variable as mutably borrowed in the current scope
// and records where the borrow started.
func (e *TypeEnv) MarkBorrowedMutAt(name string, pos ast.Position) {
	// Always mark in current environment to support lexical scoping (cleared on scope exit)
	e.borrowedMut[name] = true
	if pos.Line > 0 {
		e.borrowedMutAt[name] = &BorrowInfo{
			Line:    pos.Line,
			Column:  pos.Column,
			Mutable: true,
		}
	}
}

// ClearBorrowedMut clears a mutable borrow from the CURRENT scope only.
func (e *TypeEnv) ClearBorrowedMut(name string) {
	delete(e.borrowedMut, name)
	delete(e.borrowedMutAt, name)
}

// IsBorrowedMut checks if a variable is mutably borrowed
func (e *TypeEnv) IsBorrowedMut(name string) bool {
	if e.borrowedMut[name] {
		return true
	}
	if e.parent != nil {
		return e.parent.IsBorrowedMut(name)
	}
	return false
}

// GetBorrowedMutInfo returns origin info for an active mutable borrow.
func (e *TypeEnv) GetBorrowedMutInfo(name string) *BorrowInfo {
	if e.borrowedMut[name] {
		if info, ok := e.borrowedMutAt[name]; ok {
			return info
		}
		return nil
	}
	if e.parent != nil {
		return e.parent.GetBorrowedMutInfo(name)
	}
	return nil
}

// MarkBorrowedIm records an immutable borrow for the named variable in the CURRENT scope.
func (e *TypeEnv) MarkBorrowedIm(name string) {
	e.MarkBorrowedImAt(name, ast.Position{})
}

// MarkBorrowedImAt records an immutable borrow and, for the first active borrow
// in this scope, where it started.
func (e *TypeEnv) MarkBorrowedImAt(name string, pos ast.Position) {
	if e.borrowedIm[name] == 0 && pos.Line > 0 {
		e.borrowedImAt[name] = &BorrowInfo{
			Line:    pos.Line,
			Column:  pos.Column,
			Mutable: false,
		}
	}
	e.borrowedIm[name]++
}

// ClearBorrowedIm decrements an immutable borrow count in the CURRENT scope.
func (e *TypeEnv) ClearBorrowedIm(name string) {
	if cnt, ok := e.borrowedIm[name]; ok {
		if cnt <= 1 {
			delete(e.borrowedIm, name)
			delete(e.borrowedImAt, name)
		} else {
			e.borrowedIm[name] = cnt - 1
		}
	}
}

// IsBorrowedIm returns true if there is at least one active immutable borrow.
func (e *TypeEnv) IsBorrowedIm(name string) bool {
	if e.borrowedIm[name] > 0 {
		return true
	}
	if e.parent != nil {
		return e.parent.IsBorrowedIm(name)
	}
	return false
}

// GetBorrowedImInfo returns origin info for an active immutable borrow.
func (e *TypeEnv) GetBorrowedImInfo(name string) *BorrowInfo {
	if e.borrowedIm[name] > 0 {
		if info, ok := e.borrowedImAt[name]; ok {
			return info
		}
		return nil
	}

	if e.parent != nil {
		return e.parent.GetBorrowedImInfo(name)
	}

	return nil
}
