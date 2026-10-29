package decision

import (
	"errors"
	"fmt"
)

// APIType selects the decision API dialect: the request and response shapes
// the client speaks.
type APIType string

const (
	// APITypeTypesafe is the Typesafe decision API (POST /v1/systemone).
	APITypeTypesafe APIType = "typesafe"
	// APITypeOpenAI is the OpenAI decisions API (POST /v1/decisions).
	APITypeOpenAI APIType = "openai"
)

// Adapter translates between the canonical request and response and the
// wire format of one decision API dialect.
type Adapter interface {
	// Path is the API path of the decision endpoint, appended to the base URL.
	Path() string
	// EncodeRequest renders a canonical request in the dialect's wire format.
	// The model comes from the client config, not the request.
	EncodeRequest(req *Request, model string) (any, error)
	// DecodeResponse parses a dialect response into the canonical format.
	DecodeResponse(data []byte) (*Response, error)
}

// newAdapter returns the adapter for the given API type.
func newAdapter(apiType APIType) (Adapter, error) {
	switch apiType {
	case APITypeTypesafe:
		return Typesafe{}, nil
	case APITypeOpenAI:
		return OpenAI{}, nil
	case "":
		return nil, errors.New("decision: no API type set")
	default:
		return nil, fmt.Errorf("decision: unknown API type %q", apiType)
	}
}
