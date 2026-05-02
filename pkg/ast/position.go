package ast

import "github.com/baxromumarov/bak/pkg/token"

func tokenPosition(tok token.Token) Position {
	return Position{
		Line:   tok.Line,
		Column: tok.Column,
	}
}

func expressionPosition(expr Expression) Position {
	if isNilNode(expr) {
		return Position{}
	}
	return expr.Pos()
}

func (i *Identifier) Pos() Position {
	if i == nil {
		return Position{}
	}
	return tokenPosition(i.Token)
}

func (mi *MutableIdentifier) Pos() Position {
	if mi == nil {
		return Position{}
	}
	return tokenPosition(mi.Token)
}

func (il *IntegerLiteral) Pos() Position {
	if il == nil {
		return Position{}
	}
	return tokenPosition(il.Token)
}

func (fl *FloatLiteral) Pos() Position {
	if fl == nil {
		return Position{}
	}
	return tokenPosition(fl.Token)
}

func (fl *FStringLiteral) Pos() Position {
	if fl == nil {
		return Position{}
	}
	return tokenPosition(fl.Token)
}

func (sl *StringLiteral) Pos() Position {
	if sl == nil {
		return Position{}
	}
	return tokenPosition(sl.Token)
}

func (cl *CharLiteral) Pos() Position {
	if cl == nil {
		return Position{}
	}
	return tokenPosition(cl.Token)
}

func (bl *BooleanLiteral) Pos() Position {
	if bl == nil {
		return Position{}
	}
	return tokenPosition(bl.Token)
}

func (vl *VoidLiteral) Pos() Position {
	if vl == nil {
		return Position{}
	}
	return tokenPosition(vl.Token)
}

func (pe *PrefixExpression) Pos() Position {
	if pe == nil {
		return Position{}
	}
	return tokenPosition(pe.Token)
}

func (tc *TypeConversion) Pos() Position {
	if tc == nil {
		return Position{}
	}
	return tokenPosition(tc.Token)
}

func (vl *VecLiteral) Pos() Position {
	if vl == nil {
		return Position{}
	}
	return tokenPosition(vl.Token)
}

func (te *TupleExpression) Pos() Position {
	if te == nil {
		return Position{}
	}
	return tokenPosition(te.Token)
}

func (be *BorrowExpression) Pos() Position {
	if be == nil {
		return Position{}
	}
	return tokenPosition(be.Token)
}

func (de *DerefExpression) Pos() Position {
	if de == nil {
		return Position{}
	}
	return tokenPosition(de.Token)
}

func (fl *FunctionLiteral) Pos() Position {
	if fl == nil {
		return Position{}
	}
	return tokenPosition(fl.Token)
}
