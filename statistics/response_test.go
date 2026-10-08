package statistics

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestNewResponse(t *testing.T) {
	input, err := structpb.NewStruct(map[string]any{
		"totalCount": "42",
		"nested": map[string]any{
			"activeCount": "7",
		},
	})
	if err != nil {
		t.Fatalf("create input: %v", err)
	}

	response, err := NewResponse("example", input)
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}
	if response.GetModule() != "example" {
		t.Fatalf("module = %q, want example", response.GetModule())
	}
	if got := response.GetData().GetFields()["totalCount"].GetStringValue(); got != "42" {
		t.Fatalf("totalCount = %q, want 42", got)
	}
}

func TestNewResponseRejectsInvalidInput(t *testing.T) {
	if _, err := NewResponse("", &structpb.Struct{}); err == nil {
		t.Fatal("NewResponse() accepted an empty module name")
	}
	if _, err := NewResponse("example", nil); err == nil {
		t.Fatal("NewResponse() accepted a nil message")
	}
}
