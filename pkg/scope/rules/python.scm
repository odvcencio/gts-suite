;; Function definitions
(function_definition (identifier) @def.function)

;; Class definitions
(class_definition (identifier) @def.class)

;; Import statement: import os
(import_statement (dotted_name (identifier) @def.import))

;; From-import: from X import Y — skip the module dotted_name, capture imported name
(import_from_statement (dotted_name) (dotted_name (identifier) @def.import))

;; Variable assignments
(assignment (identifier) @def.variable)

;; References
(identifier) @ref
