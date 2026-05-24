package ast

import "testing"

func TestWalkSkipsTypedNilNodes(t *testing.T) {
	var nilBlock *BlockStatement
	var nilStmt Statement = nilBlock

	program := &Program{
		Statements: []Statement{
			nilStmt,
			&IfStatement{
				Consequence: nilBlock,
				Alternative: nilBlock,
			},
		},
	}

	var visited []Node
	Walk(program, func(node Node) {
		visited = append(visited, node)
	})

	if len(visited) != 2 {
		t.Fatalf("Walk visited %d nodes, want 2", len(visited))
	}
	if visited[0] != program {
		t.Fatalf("first visited node = %T, want *Program", visited[0])
	}
	if _, ok := visited[1].(*IfStatement); !ok {
		t.Fatalf("second visited node = %T, want *IfStatement", visited[1])
	}
}
