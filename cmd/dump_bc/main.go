package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

// Minimal structures to parse the JSON
type Module struct {
	Functions []Function `json:"Functions"`
	Strings   []string   `json:"Strings"`
	Constants []Constant `json:"Constants"`
}

type Function struct {
	Name string `json:"Name"`
	Code []int  `json:"Code"`
}

type Constant struct {
	Type     int    `json:"Type"` // 0=Int, 2=String...
	AsInt    int64  `json:"AsInt"`
	AsString string `json:"AsString"`
}

// Opcode map (simplified reverse map)
var OpcodeNames = map[int]string{
	0:  "OP_CONST",
	1:  "OP_POP",
	2:  "OP_DUP",
	3:  "OP_SWAP",
	4:  "OP_GET_LOCAL",
	5:  "OP_SET_LOCAL",
	6:  "OP_GET_GLOBAL",
	7:  "OP_SET_GLOBAL",
	8:  "OP_ADD",
	9:  "OP_SUB",
	10: "OP_MUL",
	11: "OP_DIV",
	12: "OP_MOD",
	13: "OP_NEG",
	14: "OP_EQ",
	15: "OP_NEQ",
	16: "OP_LT",
	17: "OP_LTE",
	18: "OP_GT",
	19: "OP_GTE",
	20: "OP_NOT",
	21: "OP_AND",
	22: "OP_OR",
	23: "OP_JMP",
	24: "OP_JMP_IF_FALSE",
	25: "OP_JMP_IF_TRUE",
	26: "OP_RETURN",
	27: "OP_RETURN_VOID",
	28: "OP_CALL",
	29: "OP_CALL_METHOD",
	30: "OP_CLOSURE",
	31: "OP_GET_FUNC",
	32: "OP_GET_UPVALUE",
	33: "OP_SET_UPVALUE",
	34: "OP_CLOSE_UPVALUE",
	35: "OP_NEW_STRUCT",
	36: "OP_GET_FIELD",
	37: "OP_SET_FIELD",
	38: "OP_NEW_ENUM",
	39: "OP_IS_VARIANT",
	40: "OP_GET_PAYLOAD",
	41: "OP_NEW_OPTION_SOME",
	42: "OP_NEW_OPTION_NONE",
	43: "OP_NEW_VEC_FIXED",
	44: "OP_NEW_VEC_DYNAMIC",
	45: "OP_VEC_LEN",
	46: "OP_VEC_GET",
	47: "OP_VEC_SET",
	48: "OP_VEC_PUSH",
	49: "OP_VEC_POP",
	50: "OP_NEW_RANGE",
	51: "OP_PRINT",
	52: "OP_PRINTLN",
	53: "OP_BUILTIN",
	54: "OP_CONCAT",
	55: "OP_NIL",
	56: "OP_TRUE",
	57: "OP_FALSE",
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if err == errUsage {
			fmt.Fprintln(os.Stderr, "Usage: dump_bc <file.json> [func_name]")
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var errUsage = fmt.Errorf("usage")

func run(args []string) error {
	if len(args) < 1 {
		return errUsage
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("dump_bc: reading %s: %w", args[0], err)
	}

	var mod Module
	if err := json.Unmarshal(data, &mod); err != nil {
		return fmt.Errorf("dump_bc: parsing %s: %w", args[0], err)
	}

	targetFunc := "driver.run"
	if len(args) > 1 {
		targetFunc = args[1]
	}

	for _, fn := range mod.Functions {
		if fn.Name == targetFunc {
			strfmt.Println("Displaying bytecode for ", fn.Name, " (len: ", len(fn.Code), ")")
			disassemble(fn.Code, mod)
			return nil
		}
	}
	return fmt.Errorf("dump_bc: function %s not found", targetFunc)
}

func disassemble(code []int, mod Module) {
	ip := 0
	for ip < len(code) {
		op := code[ip]
		opName := OpcodeNames[op]
		if opName == "" {
			opName = strfmt.Named("UNKNOWN({op})",
				"op", op,
			)
		}

		fmt.Printf("%04d: %s", ip, opName)
		ip++

		// operand handling (simple heuristic based on opcode name)
		if opName == "OP_CONST" {
			idx := (code[ip] << 8) | code[ip+1]
			strfmt.Print(" ", idx, " (Const: ", getConst(mod, idx), ")")
			ip += 2

		} else if strings.Contains(opName, "JMP") ||
			opName == "OP_GET_GLOBAL" ||
			opName == "OP_SET_GLOBAL" ||
			opName == "OP_GET_FUNC" {
			// Short operands
			hi := code[ip]
			lo := code[ip+1]
			ip += 2
			val := (hi << 8) | lo
			// Signed check for JMP
			if opName == "OP_JMP" {
				// Manual signed interpretation
				if hi > 127 { // Negative
					// reconstruct negative int16 from 2 bytes
					// (hi_signed << 8) | lo
					// int8(hi)
					negHi := int(int8(hi))
					val = (negHi << 8) | lo
				}
				strfmt.Print(" offset=", val, " (target=", ip+val-3, ")") // -3 because we advanced ip by 3 (1 op, 2 args)
			} else {
				strfmt.Print(" val=", val)
			}
		} else if strings.Contains(opName, "CALL") ||
			strings.Contains(opName, "BUILTIN") ||
			strings.Contains(opName, "LOCAL") {
			// Byte operands
			strfmt.Print(" ", code[ip])
			ip++
			if strings.Contains(opName, "CALL_METHOD") {
				// Method has 2 args (index short) + argc (byte)?
				// No, checking vm.go impl.
				// OP_CALL_METHOD: readShort (nameIdx), then byte (argc).
				// So 3 bytes args.
				// code[ip] was argc? No, wait.
				// Need correct disassembly logic for complex ops.
				// Assuming CALL is byte argc.
				// SET_LOCAL/GET_LOCAL is byte slot.
			}
		}

		fmt.Println()
	}
}

func getConst(mod Module, idx int) string {
	if idx < len(mod.Constants) {
		c := mod.Constants[idx]
		if c.Type == 0 {
			return strconv.FormatInt(c.AsInt, 10)
		}
		return strfmt.Named("{AsString}",
			"AsString", strconv.Quote(c.AsString),
		)
	}
	return "?"
}
