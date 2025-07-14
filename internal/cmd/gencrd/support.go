package gencrd

import "github.com/krateoplatformops/crdgen"

const (
	widgetsGroup          = "widgets.templates.krateo.io"
	preserveUnknownFields = `{"type": "object", "additionalProperties": true,"x-kubernetes-preserve-unknown-fields": true}`
)

/***************************************/
/* Custom crdgen.JsonSchemaGetter      */
/***************************************/
func fromBytes(data []byte) crdgen.JsonSchemaGetter {
	return &bytesJsonSchemaGetter{
		data: data,
	}
}

var _ crdgen.JsonSchemaGetter = (*bytesJsonSchemaGetter)(nil)

type bytesJsonSchemaGetter struct {
	data []byte
}

func (sg *bytesJsonSchemaGetter) Get() ([]byte, error) {
	return sg.data, nil
}
