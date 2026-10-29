package decision

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// openAIPath is the API path of the OpenAI decisions endpoint.
const openAIPath = "/v1/decisions"

// openAIType maps a canonical type to its OpenAI wire type. It drives
// both directions: questions are encoded with toOpenAIType, answers are
// decoded with fromOpenAIType.
var openAIType = map[string]string{
	TypeNoul:    "predicate",
	TypeChoice:  "choice",
	TypeScore:   "score",
	TypeRefusal: "refusal",
}

// OpenAI speaks the OpenAI decisions API.
type OpenAI struct{}

// Path returns the API path of the decision endpoint.
func (OpenAI) Path() string {
	return openAIPath
}

// openAIRequest is the OpenAI decisions request body.
type openAIRequest struct {
	Model     string           `json:"model"`
	Input     string           `json:"input"`
	Questions []openAIQuestion `json:"questions"`
}

// openAIQuestion is one question in an OpenAI decisions request.
type openAIQuestion struct {
	Type         string         `json:"type"`
	Name         string         `json:"name"`
	Instructions string         `json:"instructions"`
	Choices      []openAIChoice `json:"choices,omitempty"`
	Levels       []openAILevel  `json:"levels,omitempty"`
}

// openAIChoice is one option of a choice question.
type openAIChoice struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// openAILevel is one level of a score question.
type openAILevel struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// EncodeRequest renders the canonical request in the OpenAI wire format.
func (OpenAI) EncodeRequest(req *Request, model string) (any, error) {
	if model == "" {
		return nil, errors.New("decision: the openai API requires a model")
	}

	questions := make([]openAIQuestion, 0, len(req.Questions))

	for _, id := range sortedKeys(req.Questions) {
		question, err := encodeOpenAIQuestion(id, req.Questions[id])
		if err != nil {
			return nil, err
		}

		questions = append(questions, question)
	}

	return &openAIRequest{
		Model:     model,
		Input:     text(req.State),
		Questions: questions,
	}, nil
}

// encodeOpenAIQuestion converts one canonical question to the OpenAI format.
func encodeOpenAIQuestion(id string, question Question) (openAIQuestion, error) {
	out := openAIQuestion{
		Type:         toOpenAIType(question.Type),
		Name:         id,
		Instructions: text(question.Instructions),
	}

	switch question.Type {
	case TypeChoice:
		criteria, err := choiceCriteria(question.Criteria)
		if err != nil {
			return openAIQuestion{}, fmt.Errorf("decision: question %q: %w", id, err)
		}

		for _, value := range sortedKeys(criteria) {
			out.Choices = append(out.Choices, openAIChoice{
				Value:       value,
				Description: descriptionOf(criteria[value]),
			})
		}
	case TypeScore:
		levels, err := scoreLevels(question.Criteria)
		if err != nil {
			return openAIQuestion{}, fmt.Errorf("decision: question %q: %w", id, err)
		}

		out.Levels = levels
	case TypeNoul:
		// The OpenAI predicate type has no criteria field, so the true/false
		// meanings are dropped.
	default:
		return openAIQuestion{}, fmt.Errorf("decision: question %q has unknown type %q", id, question.Type)
	}

	return out, nil
}

// choiceCriteria reads the criteria of a choice question.
func choiceCriteria(criteria any) (map[string]any, error) {
	switch typed := criteria.(type) {
	case ChoiceCriteria:
		return typed, nil
	case map[string]any:
		return typed, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("has choice type but criteria of type %T", criteria)
	}
}

// scoreLevels reads the criteria of a score question as OpenAI levels.
func scoreLevels(criteria any) ([]openAILevel, error) {
	var levels []any

	switch typed := criteria.(type) {
	case ScoreCriteria:
		levels = typed
	case []any:
		levels = typed
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("has score type but criteria of type %T", criteria)
	}

	out := make([]openAILevel, 0, len(levels))

	for _, level := range levels {
		label, description := levelOf(level)

		out = append(out, openAILevel{Label: label, Description: description})
	}

	return out, nil
}

// openAIResponse is the OpenAI decisions response body.
type openAIResponse struct {
	Model   string         `json:"model"`
	Answers []openAIAnswer `json:"answers"`
	Usage   openAIUsage    `json:"usage"`
}

// openAIUsage holds the token counts of an OpenAI decisions call.
type openAIUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// openAIAnswer is one answer in an OpenAI decisions response.
type openAIAnswer struct {
	Type          string              `json:"type"`
	Name          string              `json:"name"`
	Probability   *float64            `json:"probability,omitempty"`
	Choice        json.RawMessage     `json:"choice,omitempty"`
	Score         *float64            `json:"score,omitempty"`
	Confidence    *float64            `json:"confidence,omitempty"`
	Probabilities []openAIProbability `json:"probabilities,omitempty"`
}

// openAIProbability is one entry of an answer's probability distribution.
type openAIProbability struct {
	Value       json.RawMessage `json:"value"`
	Label       string          `json:"label,omitempty"`
	Probability float64         `json:"probability"`
}

// DecodeResponse parses an OpenAI response into the canonical format.
func (OpenAI) DecodeResponse(data []byte) (*Response, error) {
	var in openAIResponse

	err := json.Unmarshal(data, &in)
	if err != nil {
		return nil, err
	}

	out := &Response{
		Model: in.Model,
		Usage: Usage{
			InputTokens:  in.Usage.InputTokens,
			OutputTokens: in.Usage.OutputTokens,
		},
		Answers: make(map[string]Answer, len(in.Answers)),
	}

	for _, answer := range in.Answers {
		out.Answers[answer.Name] = decodeOpenAIAnswer(answer)
	}

	return out, nil
}

// decodeOpenAIAnswer converts one OpenAI answer to the canonical format.
func decodeOpenAIAnswer(in openAIAnswer) Answer {
	out := Answer{Type: fromOpenAIType(in.Type), Confidence: in.Confidence}

	switch out.Type {
	case TypeNoul:
		out.Noul = in.Probability
	case TypeChoice:
		out.Choice = rawText(in.Choice)
		out.Probabilities = map[string]float64{}

		for _, probability := range in.Probabilities {
			out.Probabilities[rawText(probability.Value)] = probability.Probability
		}
	case TypeScore:
		out.Score = in.Score
		out.Probabilities = map[string]float64{}
		out.Legend = map[string]string{}

		for _, probability := range in.Probabilities {
			index := rawText(probability.Value)
			out.Probabilities[index] = probability.Probability

			label := probability.Label
			if label == "" {
				label = index
			}

			out.Legend[index] = label
		}
	}

	return out
}

// toOpenAIType maps a canonical type to the OpenAI wire type.
func toOpenAIType(canonical string) string {
	if wire, ok := openAIType[canonical]; ok {
		return wire
	}

	return canonical
}

// fromOpenAIType maps an OpenAI wire type to the canonical type.
func fromOpenAIType(wire string) string {
	for canonical, wireType := range openAIType {
		if wireType == wire {
			return canonical
		}
	}

	return wire
}

// text renders a canonical value as the plain text the OpenAI API accepts.
func text(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}

	return string(data)
}

// rawText renders a raw JSON scalar as a string.
func rawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var textValue string

	err := json.Unmarshal(raw, &textValue)
	if err == nil {
		return textValue
	}

	return string(raw)
}

// descriptionOf renders a criteria value as a description.
func descriptionOf(value any) string {
	if object, isObject := value.(map[string]any); isObject {
		if description, isString := object["description"].(string); isString {
			return description
		}
	}

	return text(value)
}

// levelOf renders a score level as a label and a description.
func levelOf(value any) (label string, description string) {
	if object, isObject := value.(map[string]any); isObject {
		if labelValue, isString := object["label"].(string); isString {
			if descriptionValue, isDescription := object["description"].(string); isDescription {
				return labelValue, descriptionValue
			}

			return labelValue, ""
		}
	}

	// A canonical score level is a description with no separate label, so the
	// text goes in both fields: the label names the level and the description
	// gives the model the rubric. Use a {label, description} level to set
	// them separately.
	return text(value), text(value)
}

// sortedKeys returns the map keys in a stable order.
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))

	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
