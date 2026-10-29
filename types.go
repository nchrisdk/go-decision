// Package decision is a client for the decision API. It speaks two API
// dialects, typesafe and openai, translating the canonical request and
// response to and from each dialect's wire format (typesafe.go, openai.go).
package decision

// Question and answer types in the canonical format.
const (
	// TypeNoul is a yes/no question.
	TypeNoul = "noul"
	// TypeChoice is a question that selects one named option.
	TypeChoice = "choice"
	// TypeScore is a question scored against ordered levels.
	TypeScore = "score"
	// TypeRefusal is an answer where the API declined to decide.
	TypeRefusal = "refusal"
)

// Request is the canonical request body. The wire format depends on the
// API type.
type Request struct {
	// State is the content being evaluated: a string, object, or array.
	State any `json:"state"`
	// Questions maps a question id to a question. Answers are keyed by the same ids.
	Questions map[string]Question `json:"questions"`
}

// Question is a single evaluation question. The shape of Criteria
// depends on Type:
//   - "noul":   *NoulCriteria
//   - "choice": ChoiceCriteria
//   - "score":  ScoreCriteria
type Question struct {
	Type string `json:"type"`
	// Instructions is the question to evaluate: a string, object, or array.
	Instructions any `json:"instructions"`
	// Criteria is the type-specific evaluation criteria (see above).
	Criteria any `json:"criteria,omitempty"`
}

// NewNoulQuestion creates a yes/no question. Criteria may be nil when the
// API's default interpretation is appropriate.
func NewNoulQuestion(instructions any, criteria *NoulCriteria) Question {
	return Question{Type: TypeNoul, Instructions: instructions, Criteria: criteria}
}

// NewChoiceQuestion creates a question that selects one named option.
func NewChoiceQuestion(instructions any, criteria ChoiceCriteria) Question {
	return Question{Type: TypeChoice, Instructions: instructions, Criteria: criteria}
}

// NewScoreQuestion creates a question scored against ordered levels.
func NewScoreQuestion(instructions any, criteria ScoreCriteria) Question {
	return Question{Type: TypeScore, Instructions: instructions, Criteria: criteria}
}

// NoulCriteria describes what "yes" and "no" mean for a noul question.
type NoulCriteria struct {
	True  any `json:"true"`
	False any `json:"false"`
}

// ChoiceCriteria maps a choice option to its rubric description.
type ChoiceCriteria map[string]any

// ScoreCriteria is an ordered list of level descriptions.
type ScoreCriteria []any

// Response is the canonical response body.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Usage holds the token counts for a call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Answer is the answer to a single question. The fields that are set
// depend on Type:
//   - "noul":   Noul
//   - "choice": Choice, Probabilities, Confidence
//   - "score":  Score, Legend, Probabilities, Confidence
type Answer struct {
	Type string `json:"type"`
	// Noul is the yes-probability, 0 (no) to 1 (yes). Set for noul answers.
	Noul *float64 `json:"noul,omitempty"`
	// Choice is the selected option. Set for choice answers.
	Choice string `json:"choice,omitempty"`
	// Score is the probability-weighted level value; can land between levels. Set for score answers.
	Score *float64 `json:"score,omitempty"`
	// Confidence is the certainty derived from the probability distribution, 0–1.
	// Set for choice and score answers.
	Confidence *float64 `json:"confidence,omitempty"`
	// Probabilities maps option (choice) or level index as string (score) to its probability.
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	// Legend maps level index (as string) to its description. Set for score answers.
	Legend map[string]string `json:"legend,omitempty"`
}
