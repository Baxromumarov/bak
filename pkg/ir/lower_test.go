package ir

import (
	"testing"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
)

type moveResolver struct{}

func (moveResolver) GetNodeType(node ast.Node) string { return "string" }
func (moveResolver) IsMoveExpression(expr ast.Expression) bool {
	id, ok := expr.(*ast.Identifier)
	return ok && id.Value == "value"
}

func TestLowerProducesOwnershipCFGAndDrops(t *testing.T) {
	p := parser.New(lexer.New(`package main
func consume(v: string) -> (void) { return void }
func demo(flag: bool, value: string) -> (void) {
    if flag { consume(value) }
    return void
}`))
	program := p.ParseProgram()
	module := LowerModule("main", program, moveResolver{})
	var moves, branches, drops int
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op {
				case OpMove:
					moves++
				case OpBranch:
					branches++
				case OpDrop:
					drops++
				}
			}
		}
	}
	if moves == 0 || branches == 0 || drops == 0 {
		t.Fatalf("expected move, branch, and drop operations; got moves=%d branches=%d drops=%d", moves, branches, drops)
	}
}

func TestLowerProducesTargetedLoopControlFlow(t *testing.T) {
	p := parser.New(lexer.New(`package main
func demo(items: Vec<int, _>, keepGoing: bool) -> (void) {
    while keepGoing {
        continue
    }
    for item in items {
        if item == 0 { break }
    }
    return void
}`))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	module := LowerModule("main", program, moveResolver{})
	var branches, iteratorStores int
	for _, fn := range module.Functions {
		if fn.Name != "demo" {
			continue
		}
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op {
				case OpJump:
					if len(inst.Targets) != 1 {
						t.Fatalf("loop jump has no explicit target: %#v", inst)
					}
				case OpBranch:
					branches++
				case OpStore:
					if inst.Place != nil && inst.Place.Name == "item" {
						iteratorStores++
					}
				}
			}
		}
	}
	if branches < 3 || iteratorStores != 1 {
		t.Fatalf("expected while/for/if CFG and iterator storage; branches=%d iterator stores=%d", branches, iteratorStores)
	}
}

func TestLowerDoesNotContinueAfterFullyTerminatingIf(t *testing.T) {
	p := parser.New(lexer.New(`package main
func demo(flag: bool) -> (int) {
    if flag { return 1 } else { return 2 }
    var unreachable: int = 3
    return unreachable
}`))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	module := LowerModule("main", program, moveResolver{})
	for _, fn := range module.Functions {
		if fn.Name != "demo" {
			continue
		}
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if inst.Place != nil && inst.Place.Name == "unreachable" {
					t.Fatalf("lowered unreachable storage after terminating if: %#v", inst)
				}
			}
		}
	}
}
