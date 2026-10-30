package spec

import _ "embed"

// Schema is a JSON Schema for pinfold.yml, for editor support. The decoder,
// not the schema, is the authority; a test keeps the two in step.
//
//go:embed schema/pinfold.schema.json
var Schema []byte
