package gateway

import (
	"encoding/json"
	"testing"
)

func TestReviewedSchemaPreservesNumbersAndDeniesRemoteReferences(t *testing.T) {
	for _, test := range []struct {
		schema, value string
		valid         bool
	}{
		{`{"type":"object","required":["amount"],"properties":{"amount":{"type":"integer","const":9007199254740993}},"additionalProperties":false}`, `{"amount":9007199254740993}`, false},
		{`{"type":"object","required":["amount"],"properties":{"amount":{"type":"integer","const":9007199254740993}},"additionalProperties":false}`, `{"amount":9007199254740992}`, false},
		{`{"type":"object","properties":{"amount":{"type":"integer"}}}`, `{"amount":9007199254740993}`, true},
		{`{"$ref":"https://attacker.invalid/schema"}`, `{}`, false},
		{`{"type":"object","required":["amount"]}`, `{"taskId":"original-task"}`, false},
	} {
		if err := ValidateReviewedSchema(json.RawMessage(test.schema), json.RawMessage(test.value)); (err == nil) != test.valid {
			t.Fatalf("expected valid=%v err=%v", test.valid, err)
		}
	}
}
