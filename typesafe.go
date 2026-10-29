package decision

import (
	"encoding/json"
)

// typesafePath is the API path of the typesafe decision endpoint.
const typesafePath = "/v1/systemone"

// Typesafe speaks the typesafe decision API.
type Typesafe struct{}

// Path returns the API path of the decision endpoint.
func (Typesafe) Path() string {
	return typesafePath
}

// typesafeRequest is the typesafe decision request body.
type typesafeRequest struct {
	// State is the content being evaluated.
	State any `json:"state"`
	// Model is the model the API uses.
	Model string `json:"model"`
	// Questions maps a question id to a question.
	Questions map[string]Question `json:"questions"`
}

// EncodeRequest renders the canonical request in the typesafe wire format.
func (Typesafe) EncodeRequest(req *Request, model string) (any, error) {
	return &typesafeRequest{
		State:     req.State,
		Model:     model,
		Questions: req.Questions,
	}, nil
}

// DecodeResponse parses a typesafe response into the canonical format.
func (Typesafe) DecodeResponse(data []byte) (*Response, error) {
	var out Response

	err := json.Unmarshal(data, &out)
	if err != nil {
		return nil, err
	}

	// Normalize so that a body without "answers" has the same shape as the
	// OpenAI decoder: an empty map, never nil.
	if out.Answers == nil {
		out.Answers = map[string]Answer{}
	}

	return &out, nil
}
