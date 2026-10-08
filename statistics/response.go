// Package statistics provides adapters for the shared module statistics protocol.
package statistics

import (
	"fmt"

	commonV1 "github.com/go-tangra/go-tangra-common/gen/go/common/service/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// NewResponse converts a module-specific protobuf response into the stable
// common statistics envelope. Proto JSON names and 64-bit integer encoding are
// preserved so existing HTTP clients continue to receive the same data shape.
func NewResponse(module string, message proto.Message) (*commonV1.GetModuleStatisticsResponse, error) {
	if module == "" {
		return nil, fmt.Errorf("statistics module name is required")
	}
	if message == nil {
		return nil, fmt.Errorf("statistics response for module %q is nil", module)
	}

	payload, err := protojson.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal statistics response for module %q: %w", module, err)
	}

	data := &structpb.Struct{}
	if err := protojson.Unmarshal(payload, data); err != nil {
		return nil, fmt.Errorf("convert statistics response for module %q: %w", module, err)
	}

	return &commonV1.GetModuleStatisticsResponse{
		Module: module,
		Data:   data,
	}, nil
}
