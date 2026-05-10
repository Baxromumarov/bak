package main

import (
	"encoding/json"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/formatter"
)

func collectInlayHints(text string, result *AnalysisResult, _ *Server) []InlayHint {
	hints := []InlayHint{}
	lines := strings.Split(text, "\n")
	var walk func(node ast.Node)
	walk = func(node ast.Node) {
		if node == nil {
			return
		}
		// log.Printf("Visiting node: %T", node)
		switch n := node.(type) {
		case *ast.CallExpression:
			if n == nil {
				return
			}
			fnName := ""
			if ident, ok := n.Function.(*ast.Identifier); ok && ident != nil {
				fnName = ident.Value
			}
			var sig SignatureInfo
			if fnName != "" && result.Index != nil {
				if s, ok := result.Index.Sigs[fnName]; ok {
					sig = s
				}
			}
			if sig.Label != "" {
				for i, arg := range n.Arguments {
					if arg == nil {
						continue
					}
					if i >= len(sig.Params) {
						break
					}
					pos := exprStartPosition(arg)
					if pos.Line >= 0 {
						label := sig.Params[i]
						if idx := strings.Index(label, ":"); idx != -1 {
							label = strings.TrimSpace(label[:idx]) + ":"
						}
						hints = append(hints, InlayHint{
							Position:     positionFromLineCol(pos.Line, pos.Col),
							Label:        label,
							Kind:         1,
							PaddingRight: true,
						})
					}
				}
			}
			walk(n.Function)
			for _, arg := range n.Arguments {
				if arg == nil {
					continue
				}
				walk(arg)
			}
		case *ast.VarStatement:
			if n != nil {
				if n.Value != nil {
					walk(n.Value)
				}
				// Add type hint if type is implicit (check for presence of colon in source)
				if n.Name != nil && n.Value != nil && result.TC != nil {
					// Check if explicit type annotation exists by looking for colon after var name
					nameStart := positionFromLineCol(n.Name.Token.Line, n.Name.Token.Column)
					nameEndLine := nameStart.Line
					nameEndCol := nameStart.Character + len(n.Name.Value)

					// Find start of value
					valPos := exprStartPosition(n.Value)
					if valPos.Line > 0 {
						valStart := positionFromLineCol(valPos.Line, valPos.Col)
						valStartLine := valStart.Line
						valStartCol := valStart.Character

						isExplicit := false

						// Only support checking single-line declarations for now
						if nameEndLine == valStartLine && nameEndLine < len(lines) {
							line := lines[nameEndLine]
							// Ensure bounds
							if nameEndCol < len(line) && valStartCol <= len(line) && nameEndCol < valStartCol {
								segment := line[nameEndCol:valStartCol]
								if strings.Contains(segment, ":") {
									isExplicit = true
								}
							}
						} else {
							// If multi-line, assume explicit to be safe
							if nameEndLine != valStartLine {
								isExplicit = true
							}
						}

						if !isExplicit {
							if t := strings.TrimSpace(result.TC.GetNodeType(n.Name)); t != "" && t != "void" {
								hints = append(hints, InlayHint{
									Position:     positionFromLineCol(n.Name.Token.Line, n.Name.Token.Column+len(n.Name.Value)),
									Label:        ": " + t,
									Kind:         2,
									PaddingLeft:  false,
									PaddingRight: false,
								})
							}
						}
					}
				}
			}
		case *ast.MethodCallExpression:
			if n == nil {
				return
			}
			walk(n.Object)
			for _, arg := range n.Arguments {
				if arg == nil {
					continue
				}
				walk(arg)
			}
		case *ast.ExpressionStatement:
			if n == nil {
				return
			}
			walk(n.Expression)
		case *ast.Program:
			if n == nil {
				return
			}
			for _, stmt := range n.Statements {
				if stmt != nil {
					walk(stmt)
				}
			}
		case *ast.BlockStatement:
			if n == nil {
				return
			}
			for _, stmt := range n.Statements {
				if stmt != nil {
					walk(stmt)
				}
			}
		case *ast.IfStatement:
			if n == nil {
				return
			}
			walk(n.Condition)
			walk(n.Consequence)
			walk(n.Alternative)
		case *ast.WhileStatement:
			if n == nil {
				return
			}
			walk(n.Condition)
			walk(n.Body)
		case *ast.ForStatement:
			if n == nil {
				return
			}
			walk(n.Iterable)
			walk(n.Body)
		case *ast.SwitchStatement:
			if n == nil {
				return
			}
			walk(n.Value)
			for _, c := range n.Cases {
				if c == nil {
					continue
				}
				for _, expr := range c.Values {
					if expr == nil {
						continue
					}
					walk(expr)
				}
				walk(c.Body)
			}
		case *ast.FunctionDecl:
			if n == nil || n.Body == nil {
				return
			}
			walk(n.Body)
		case *ast.ImplDecl:
			if n == nil {
				return
			}
			for _, m := range n.Methods {
				if m == nil || m.Body == nil {
					continue
				}
				walk(m.Body)
			}
		}
	}
	walk(result.AST)
	return hints
}

type exprPos struct {
	Line int
	Col  int
}

func exprStartPosition(expr ast.Expression) exprPos {
	if expr == nil {
		return exprPos{}
	}
	pos := expr.Pos()
	return exprPos{Line: pos.Line, Col: pos.Column}
}

func (s *Server) handleSignatureHelp(req Request) *SignatureHelp {
	var params SignatureHelpParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}

	text, ok := s.document(params.TextDocument.URI)
	if !ok {
		return nil
	}

	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok || result == nil {
		return nil
	}

	return buildSignatureHelp(
		params.TextDocument.URI,
		text,
		params.Position,
		result,
		s,
	)
}

func (s *Server) handleFormatting(req Request) []TextEdit {
	var params DocumentFormattingParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}
	text, ok := s.document(params.TextDocument.URI)
	if !ok {
		return nil
	}
	formatted, errs := formatter.Format(text)
	if len(errs) > 0 {
		return nil
	}
	return []TextEdit{
		{
			Range:   fullDocumentRange(text),
			NewText: formatted,
		},
	}
}

func (s *Server) handleSemanticTokensFull(_ Request) *SemanticTokens {
	// Disable server-side semantic tokens. Returning an empty token
	// set forces the client to fall back to TextMate grammar + theme
	// for syntax coloring and avoids theme mismatches.
	return &SemanticTokens{Data: []int{}}
}

func (s *Server) handleInlayHint(req Request) []InlayHint {
	var params InlayHintParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil
	}

	defer recoverAndLog("textDocument/inlayHint")

	text, ok := s.document(params.TextDocument.URI)
	if !ok {
		return nil
	}

	result, ok := s.analysisResult(params.TextDocument.URI)
	if !ok ||
		result == nil ||
		result.AST == nil ||
		result.Index == nil {
		return nil
	}

	return collectInlayHints(text, result, s)
}
