package ast

import "github.com/baxromumarov/bak/pkg/token"

func tokenPosition(tok token.Token) Position {
	return Position{Line: tok.Line, Column: tok.Column}
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

func (ie *InfixExpression) Pos() Position {
	if ie == nil {
		return Position{}
	}
	if pos := expressionPosition(ie.Left); pos != (Position{}) {
		return pos
	}
	return tokenPosition(ie.Token)
}

func (ce *CallExpression) Pos() Position {
	if ce == nil {
		return Position{}
	}
	if pos := expressionPosition(ce.Function); pos != (Position{}) {
		return pos
	}
	return tokenPosition(ce.Token)
}

func (tc *TypeConversion) Pos() Position {
	if tc == nil {
		return Position{}
	}
	return tokenPosition(tc.Token)
}

func (mc *MethodCallExpression) Pos() Position {
	if mc == nil {
		return Position{}
	}
	if pos := expressionPosition(mc.Object); pos != (Position{}) {
		return pos
	}
	return tokenPosition(mc.Token)
}

func (fa *FieldAccessExpression) Pos() Position {
	if fa == nil {
		return Position{}
	}
	if pos := expressionPosition(fa.Object); pos != (Position{}) {
		return pos
	}
	return tokenPosition(fa.Token)
}

func (ie *IndexExpression) Pos() Position {
	if ie == nil {
		return Position{}
	}
	if pos := expressionPosition(ie.Left); pos != (Position{}) {
		return pos
	}
	return tokenPosition(ie.Token)
}

func (sl *StructLiteral) Pos() Position {
	if sl == nil {
		return Position{}
	}
	if sl.Name != nil {
		if pos := sl.Name.Pos(); pos != (Position{}) {
			return pos
		}
	}
	return tokenPosition(sl.Token)
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

func (re *RangeExpression) Pos() Position {
	if re == nil {
		return Position{}
	}
	if pos := expressionPosition(re.Start); pos != (Position{}) {
		return pos
	}
	return tokenPosition(re.Token)
}

func (ev *EnumVariantExpression) Pos() Position {
	if ev == nil {
		return Position{}
	}
	if ev.Variant != nil {
		if pos := ev.Variant.Pos(); pos != (Position{}) {
			return pos
		}
	}
	return tokenPosition(ev.Token)
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

func (ue *UnwrapExpression) Pos() Position {
	if ue == nil {
		return Position{}
	}
	if pos := expressionPosition(ue.Value); pos != (Position{}) {
		return pos
	}
	return tokenPosition(ue.Token)
}

func (fl *FunctionLiteral) Pos() Position {
	if fl == nil {
		return Position{}
	}
	return tokenPosition(fl.Token)
}
