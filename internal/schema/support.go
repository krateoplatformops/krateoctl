package schema

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/jsonpointer"
	"github.com/krateoplatformops/krateoctl/internal/util"
)

// castNodeValueByType attempts to convert a raw string value into the appropriate type based on
// the provided fieldType. It handles boolean, integer, and number conversions. If the conversion
// fails or the type is not supported (e.g., string), it returns the original raw value.
//
// Parameters:
//   - rawValue: The string value to be converted
//   - fieldType: Array of allowed JSON Schema types for this field
//
// Returns:
//   - The converted value as any, or the original string if conversion fails/isn't needed
func castNodeValueByType(rawValue string, fieldType StringOrArrayOfString) any {
	if len(fieldType) == 0 {
		return rawValue
	}

	// rawValue must be one of fielTypes
	for _, t := range fieldType {
		switch t {
		case "boolean":
			switch rawValue {
			case "true":
				return true
			case "false":
				return false
			}
		case "integer":
			v, err := strconv.Atoi(rawValue)
			if err == nil {
				return v
			}
		case "number":
			v, err := strconv.ParseFloat(rawValue, 64)
			if err == nil {
				return v
			}
		}
	}

	return rawValue
}

// handleSchemaRefs processes and resolves JSON Schema references ($ref) within a schema.
// It handles both direct schema references and references within patternProperties.
// For each reference:
// - If it's a relative file path, it attempts to load and parse the referenced schema
// - If it includes a JSON pointer (#/path/to/schema), it extracts the specific schema section
// - The resolved schema replaces the original reference
//
// Parameters:
//   - schema: Pointer to the Schema object containing the references to resolve
//   - valuesPath: Path to the current values file, used for resolving relative paths
//
// The function will log.Fatal on any critical errors (file not found, invalid JSON, etc.)
// and log.Debug for non-critical issues (e.g., non-relative paths that may be handled elsewhere)
func handleSchemaRefs(schema *Schema, valuesPath string) {
	// Handle main schema $ref
	if schema.Ref != "" {
		refParts := strings.Split(schema.Ref, "#")
		relFilePath, err := util.IsRelativeFile(valuesPath, refParts[0])
		if err != nil {
			log.Printf("debug: %v", err)
			return
		}
		var relSchema Schema
		file, err := os.Open(relFilePath)
		if err == nil {
			defer file.Close()
			byteValue, _ := io.ReadAll(file)

			if len(refParts) > 1 {
				// Found json-pointer
				var obj any
				json.Unmarshal(byteValue, &obj)
				jsonPointerResultRaw, err := jsonpointer.Get(obj, refParts[1])
				if err != nil {
					log.Fatal(err)
				}
				jsonPointerResultMarshaled, err := json.Marshal(jsonPointerResultRaw)
				if err != nil {
					log.Fatal(err)
				}
				err = json.Unmarshal(jsonPointerResultMarshaled, &relSchema)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				// No json-pointer
				err = json.Unmarshal(byteValue, &relSchema)
				if err != nil {
					log.Fatal(err)
				}
			}
			*schema = relSchema
			schema.HasData = true
		} else {
			log.Fatal(err)
		}
	}

	// Handle $ref in pattern properties
	if schema.PatternProperties != nil {
		for pattern, subSchema := range schema.PatternProperties {
			if subSchema.Ref != "" {
				handleSchemaRefs(subSchema, valuesPath)
				schema.PatternProperties[pattern] = subSchema // Update the original schema in the map
			}
		}
	}
}
