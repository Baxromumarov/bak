// A deliberately permissive grammar.  The compiler/LSP remains the authority
// for Bak syntax and type diagnostics; this grammar gives Zed resilient
// incremental parsing, highlighting, brackets, indentation, and symbols even
// while a file is being edited.

module.exports = grammar({
  name: "bak",

  extras: $ => [
    /[\s\uFEFF\u2060\u200B]/,
    $.line_comment,
    $.block_comment,
  ],

  word: $ => $.identifier,

  rules: {
    source_file: $ => repeat($._item),

    _item: $ => choice(
      $.function_declaration,
      $.struct_declaration,
      $.enum_declaration,
      $.impl_declaration,
      $.block,
      $._token,
    ),

    function_declaration: $ => prec(1, seq(
      optional("pub"),
      optional("mut"),
      "func",
      field("name", $.identifier),
      repeat($._token),
      field("body", $.block),
    )),

    struct_declaration: $ => prec(1, seq(
      optional("pub"),
      "struct",
      field("name", $.identifier),
      repeat($._token),
      field("body", $.block),
    )),

    enum_declaration: $ => prec(1, seq(
      optional("pub"),
      "enum",
      field("name", $.identifier),
      repeat($._token),
      field("body", $.block),
    )),

    impl_declaration: $ => prec(1, seq(
      "impl",
      field("type", $.identifier),
      repeat($._token),
      field("body", $.block),
    )),

    block: $ => seq("{", repeat($._item), "}"),

    _token: $ => choice(
      $.keyword,
      $.type_name,
      $.result_constructor,
      $.identifier,
      $.string_literal,
      $.raw_string_literal,
      $.char_literal,
      $.float_literal,
      $.integer_literal,
      $.boolean_literal,
      $.operator,
      $.punctuation,
    ),

    identifier: _ => /[A-Za-z_][A-Za-z0-9_]*/,
    keyword: _ => choice(
      "package", "import", "as", "pub", "func", "trace", "var", "const", "mut",
      "struct", "enum", "impl", "if", "else", "while", "for", "in", "switch",
      "case", "default", "break", "continue", "return", "defer", "panic", "unsafe",
      "try", "type", "alias",
    ),
    type_name: _ => choice(
      "any", "byte", "int", "int8", "int16", "int32", "int64", "uint", "uint8",
      "uint16", "uint32", "uint64", "float32", "float64", "bool", "char", "string",
      "void", "Vec", "Result",
    ),
    result_constructor: _ => choice("Ok", "Err"),
    boolean_literal: _ => choice("true", "false"),
    integer_literal: _ => /(?:0[xX][0-9A-Fa-f_]+|0[bB][01_]+|[0-9][0-9_]*)/,
    float_literal: _ => /(?:[0-9][0-9_]*\.[0-9_]*(?:[eE][+-]?[0-9_]+)?|[0-9][0-9_]*[eE][+-]?[0-9_]+)/,
    string_literal: _ => token(seq(optional("f"), '"', repeat(choice(/[^"\\\n]+/, /\\./)), '"')),
    raw_string_literal: _ => token(seq("`", repeat(/[^`]/), "`")),
    char_literal: _ => token(seq("'", choice(/[^'\\\n]/, /\\./), "'")),
    line_comment: _ => token(seq("//", /[^\n]*/)),
    block_comment: _ => token(/\/\*([^*]|\*[^/])*\*\//),

    operator: _ => choice(
      "->", "..", "==", "!=", "<=", ">=", "&&", "||", "<<", ">>",
      "=", "+", "-", "!", "*", "/", "%", "<", ">", "&", "|", "^", "~", "?",
    ),
    punctuation: _ => choice("(", ")", "[", "]", ",", ";", ":", "."),
  },
});
