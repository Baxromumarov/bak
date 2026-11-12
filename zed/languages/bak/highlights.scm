; Comments and literals
[
  (line_comment)
  (block_comment)
] @comment

[
  (string_literal)
  (raw_string_literal)
] @string

(char_literal) @string.special
(integer_literal) @number
(float_literal) @number
(boolean_literal) @boolean

; Declarations contribute richer names for the outline and syntax coloring.
(function_declaration name: (identifier) @function)
(struct_declaration name: (identifier) @type)
(enum_declaration name: (identifier) @type)
(impl_declaration type: (identifier) @type)

; All remaining names are variables until the server supplies semantic tokens.
(identifier) @variable

; Keywords
[
  "package"
  "import"
  "as"
  "pub"
  "func"
  "struct"
  "enum"
  "impl"
  "type"
  "alias"
  "var"
  "const"
  "mut"
  "unsafe"
  "trace"
] @keyword

[
  "if"
  "else"
  "while"
  "for"
  "in"
  "switch"
  "case"
  "default"
  "break"
  "continue"
  "return"
  "defer"
  "panic"
  "try"
] @keyword.control

[
  "int"
  "int8"
  "int16"
  "int32"
  "int64"
  "uint"
  "uint8"
  "uint16"
  "uint32"
  "uint64"
  "byte"
  "float32"
  "float64"
  "bool"
  "char"
  "string"
  "any"
  "void"
  "Vec"
  "Result"
] @type.builtin

[
  "Ok"
  "Err"
] @constructor

(operator) @operator

[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

[
  ","
  ";"
  ":"
  "."
] @punctuation.delimiter
