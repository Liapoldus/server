package schema

// Definition is the declarative JSON Definition vocabulary used by Server contracts.
// JSON-valued keywords preserve union types, false, zero and empty arrays.
type Definition struct {
	Definition           string                `json:"$schema,omitempty"`
	ID                   string                `json:"$id,omitempty"`
	Ref                  string                `json:"$ref,omitempty"`
	Title                string                `json:"title,omitempty"`
	Description          string                `json:"description,omitempty"`
	Format               string                `json:"format,omitempty"`
	Pattern              string                `json:"pattern,omitempty"`
	MinLength            *int                  `json:"minLength,omitempty"`
	MaxLength            *int                  `json:"maxLength,omitempty"`
	Minimum              *int                  `json:"minimum,omitempty"`
	Maximum              *int                  `json:"maximum,omitempty"`
	MinItems             *int                  `json:"minItems,omitempty"`
	MaxItems             *int                  `json:"maxItems,omitempty"`
	MinProperties        *int                  `json:"minProperties,omitempty"`
	Type                 any                   `json:"type,omitempty"`
	Const                any                   `json:"const,omitempty"`
	Default              any                   `json:"default,omitempty"`
	Enum                 []any                 `json:"enum,omitempty"`
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
	UniqueItems          *bool                 `json:"uniqueItems,omitempty"`
	Required             *[]string             `json:"required,omitempty"`
	Properties           map[string]Definition `json:"properties,omitempty"`
	Defs                 map[string]Definition `json:"$defs,omitempty"`
	Items                *Definition           `json:"items,omitempty"`
	PropertyNames        *Definition           `json:"propertyNames,omitempty"`
	If                   *Definition           `json:"if,omitempty"`
	Then                 *Definition           `json:"then,omitempty"`
	Else                 *Definition           `json:"else,omitempty"`
	Not                  *Definition           `json:"not,omitempty"`
	OneOf                []Definition          `json:"oneOf,omitempty"`
	AllOf                []Definition          `json:"allOf,omitempty"`
}

func Value[T any](value T) *T { return &value }
