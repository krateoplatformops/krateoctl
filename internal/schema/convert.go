package schema

import (
	"log"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"
)

// FromYAML recursively parses a YAML node and creates a JSON Schema from it
// Parameters:
//   - valuesPath: path to the values file being processed
//   - node: current YAML node being processed
//   - parentRequiredProperties: list of required properties to populate in parent
func FromYAML(
	valuesPath string,
	node *yaml.Node,
	parentRequiredProperties *[]string,
) *Schema {
	schema := NewSchema("object")

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			log.Fatalf("Strange yaml document found:\n%v\n", node.Content[:])
		}

		schema.Schema = "http://json-schema.org/draft-07/schema#"
		schema.Properties = FromYAML(
			valuesPath,
			node.Content[0],
			&schema.Required.Strings,
		).Properties

		schema.AdditionalProperties = new(bool)

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if valueNode.Kind == yaml.AliasNode {
				valueNode = valueNode.Alias
			}

			comment := keyNode.HeadComment
			/*
				if !keepFullComment {
					leadingCommentsRemover := regexp.MustCompile(`(?s)(?m)(?:.*\n{2,})+`)
					comment = leadingCommentsRemover.ReplaceAllString(comment, "")
				}*/

			keyNodeSchema, description, err := GetSchemaFromComment(comment)
			if err != nil {
				log.Fatalf("Error while parsing comment of key %s: %v", keyNode.Value, err)
			}

			if keyNodeSchema.Ref != "" || len(keyNodeSchema.PatternProperties) > 0 {
				// Handle $ref in main schema and pattern properties
				handleSchemaRefs(&keyNodeSchema, valuesPath)
			}

			if keyNodeSchema.HasData {
				if err := keyNodeSchema.Validate(); err != nil {
					log.Fatalf(
						"Error while validating jsonschema of key %s: %v",
						keyNode.Value,
						err,
					)
				}
			} else {
				nodeType, err := typeFromTag(valueNode.Tag)
				if err != nil {
					log.Fatal(err)
				}
				keyNodeSchema.Type = nodeType
			}

			// only validate or default if $ref is not set
			if keyNodeSchema.Ref == "" {

				// Add key to required array of parent
				if keyNodeSchema.Required.Bool || (len(keyNodeSchema.Required.Strings) == 0 && !keyNodeSchema.HasData) {
					if !slices.Contains(*parentRequiredProperties, keyNode.Value) {
						*parentRequiredProperties = append(*parentRequiredProperties, keyNode.Value)
					}
				}

				if valueNode.Kind == yaml.MappingNode &&
					(!keyNodeSchema.HasData || keyNodeSchema.AdditionalProperties == nil) {
					keyNodeSchema.AdditionalProperties = new(bool)
				}

				// If no title was set, use the key value
				if keyNodeSchema.Title == "" {
					keyNodeSchema.Title = keyNode.Value
				}

				// If no description was set, use the rest of the comment as description
				if keyNodeSchema.Description == "" {
					keyNodeSchema.Description = description
				}

				// If no default value was set, use the values node value as default
				if keyNodeSchema.Default == nil && valueNode.Kind == yaml.ScalarNode {
					keyNodeSchema.Default = castNodeValueByType(valueNode.Value, keyNodeSchema.Type)
				}

				// If the value is another map and no properties are set, get them from default values
				if valueNode.Kind == yaml.MappingNode && keyNodeSchema.Properties == nil {
					// Initialize properties map if needed
					if keyNodeSchema.Properties == nil {
						keyNodeSchema.Properties = make(map[string]*Schema)
					}

					generatedProperties := FromYAML(
						valuesPath,
						valueNode,
						&keyNodeSchema.Required.Strings,
					).Properties

					// Process each property
					for i := 0; i < len(valueNode.Content); i += 2 {
						propKeyNode := valueNode.Content[i]
						// propValueNode := valueNode.Content[i+1]

						// Check if this specific property matches any pattern
						skipProperty := false
						for pattern := range keyNodeSchema.PatternProperties {
							matched, err := regexp.MatchString(pattern, propKeyNode.Value)
							if err != nil {
								log.Fatalf("Invalid pattern '%s' in patternProperties: %v", pattern, err)
							}
							if matched {
								skipProperty = true
								break
							}
						}

						// Only add schema for non-skipped properties
						if !skipProperty {
							keyNodeSchema.Properties[propKeyNode.Value] = generatedProperties[propKeyNode.Value]
						}
					}
				} else if valueNode.Kind == yaml.SequenceNode && keyNodeSchema.Items == nil {
					// If the value is a sequence, but no items are predefined
					seqSchema := NewSchema("")

					for _, itemNode := range valueNode.Content {
						if itemNode.Kind == yaml.ScalarNode {
							itemNodeType, err := typeFromTag(itemNode.Tag)
							if err != nil {
								log.Fatal(err)
							}
							seqSchema.AnyOf = append(seqSchema.AnyOf, NewSchema(itemNodeType[0]))
						} else {
							itemRequiredProperties := []string{}
							itemSchema := FromYAML(valuesPath, itemNode, &itemRequiredProperties)
							itemSchema.Required.Strings = append(itemSchema.Required.Strings, itemRequiredProperties...)

							if itemNode.Kind == yaml.MappingNode && (!itemSchema.HasData || itemSchema.AdditionalProperties == nil) {
								itemSchema.AdditionalProperties = new(bool)
							}

							seqSchema.AnyOf = append(seqSchema.AnyOf, itemSchema)
						}
					}
					keyNodeSchema.Items = seqSchema

					// Because the `required` field isn't valid jsonschema (but just a helper boolean)
					// we must convert them to valid requiredProperties fields
					FixRequiredProperties(&keyNodeSchema)
				}
			}

			if schema.Properties == nil {
				schema.Properties = make(map[string]*Schema)
			}
			schema.Properties[keyNode.Value] = &keyNodeSchema
		}
	}

	return schema
}
