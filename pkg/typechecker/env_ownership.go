package typechecker

import "github.com/baxromumarov/bak/pkg/ast"

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

// GetMoveInfo returns the move info for a variable
func (e *TypeEnv) GetMoveInfo(name string) *MoveInfo {
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
