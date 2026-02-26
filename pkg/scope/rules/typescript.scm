;; Function declarations
(function_declaration (identifier) @def.function)

;; Class declarations (uses type_identifier, not identifier)
(class_declaration (type_identifier) @def.class)

;; Interface declarations
(interface_declaration (type_identifier) @def.interface)

;; Type alias declarations
(type_alias_declaration (type_identifier) @def.type)

;; Enum declarations
(enum_declaration (identifier) @def.type)

;; Variable declarations (const/let/var)
(variable_declarator (identifier) @def.variable)

;; Import specifiers: import { useState } from 'react'
(import_specifier (identifier) @def.import)

;; Method definitions
(method_definition (property_identifier) @def.method)

;; References
(identifier) @ref
(type_identifier) @ref
