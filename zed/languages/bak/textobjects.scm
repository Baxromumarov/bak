(function_declaration
  body: (block
    "{"
    (_)* @function.inside
    "}")) @function.around

[
  (struct_declaration body: (block) @class.inside)
  (enum_declaration body: (block) @class.inside)
  (impl_declaration body: (block) @class.inside)
] @class.around

[
  (line_comment)
  (block_comment)
] @comment.around
