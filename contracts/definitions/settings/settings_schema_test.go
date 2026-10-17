package settings

import (
	"encoding/json"
	"testing"
)

func TestHTTPJSONSchemaFieldsAreMarkedAsWholeDocuments(t *testing.T) {
	contents, err := json.Marshal(SettingsSchema())
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Defs map[string]struct {
			Properties map[string]struct {
				StudioJSONDocument bool `json:"x-studio-json-document"`
				StudioJSONValue    bool `json:"x-studio-json-value"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	for definition, field := range map[string]string{
		"apiRequestBody": "schema",
		"apiResponse":    "schema",
	} {
		if !document.Defs[definition].Properties[field].StudioJSONDocument {
			t.Errorf("%s.%s is not declared as a replaceable JSON document", definition, field)
		}
	}
	if !document.Defs["apiResponse"].Properties["example"].StudioJSONValue {
		t.Error("apiResponse.example is not declared as a replaceable JSON value")
	}
}
