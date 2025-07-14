package schema

import (
	"bufio"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaPrefix and CommentPrefix define the markers used for schema annotations in comments
const (
	SchemaPrefix  = "# @schema"
	CommentPrefix = "#"

	// CustomAnnotationPrefix marks custom annotations.
	// Custom annotations are extensions to the JSON Schema specification
	// See: https://json-schema.org/blog/posts/custom-annotations-will-continue
	CustomAnnotationPrefix = "x-"
)

// GetSchemaFromComment parses the annotations from the given comment
func GetSchemaFromComment(comment string) (Schema, string, error) {
	var result Schema
	scanner := bufio.NewScanner(strings.NewReader(comment))
	description := []string{}
	rawSchema := []string{}
	insideSchemaBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, SchemaPrefix) {
			insideSchemaBlock = !insideSchemaBlock
			continue
		}
		if insideSchemaBlock {
			content := strings.TrimPrefix(line, CommentPrefix)
			rawSchema = append(rawSchema, strings.TrimPrefix(strings.TrimPrefix(content, CommentPrefix), " "))
			result.Set()
		} else {
			description = append(description, strings.TrimPrefix(strings.TrimPrefix(line, CommentPrefix), " "))
		}
	}

	if insideSchemaBlock {
		return result, "",
			fmt.Errorf("unclosed schema block found in comment: %s", comment)
	}

	err := yaml.Unmarshal([]byte(strings.Join(rawSchema, "\n")), &result)
	if err != nil {
		return result, "", err
	}

	return result, strings.Join(description, "\n"), nil
}
