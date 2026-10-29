package decision_test

import (
	"encoding/json"
	"testing"

	"github.com/nchrisdk/go-decision"
)

func TestOpenAI_EncodeRequest(t *testing.T) {
	t.Parallel()

	body := encode(t, decision.OpenAI{}, allTypesRequest(), "gpt-6-luna")

	if body["model"] != "gpt-6-luna" {
		t.Errorf("model = %v, want the configured model", body["model"])
	}

	if body["input"] != testState {
		t.Errorf("input = %v, want %q", body["input"], testState)
	}

	questions, isSlice := body["questions"].([]any)
	if !isSlice || len(questions) != 3 {
		t.Fatalf("questions = %v, want 3 questions", body["questions"])
	}

	byName := map[string]map[string]any{}

	for _, question := range questions {
		object, isObject := question.(map[string]any)
		if !isObject {
			t.Fatalf("question is not an object: %v", question)
		}

		name, isString := object["name"].(string)
		if !isString {
			t.Fatalf("question has no name: %v", object)
		}

		byName[name] = object
	}

	assertPredicate(t, byName["is_urgent"])
	assertChoice(t, byName["department"])
	assertScore(t, byName["frustration"])
}

func TestOpenAI_EncodeRequest_NoModel(t *testing.T) {
	t.Parallel()

	_, err := decision.OpenAI{}.EncodeRequest(allTypesRequest(), "")
	if err == nil {
		t.Error("want an error when no model is set")
	}
}

func TestOpenAI_DecodeResponse(t *testing.T) {
	t.Parallel()

	resp, err := decision.OpenAI{}.DecodeResponse([]byte(openAIResponseBody))
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}

	if resp.Model != "gpt-6-luna" {
		t.Errorf("model = %q, want %q", resp.Model, "gpt-6-luna")
	}

	urgent := resp.Answers["is_urgent"]
	if urgent.Type != decision.TypeNoul || urgent.Noul == nil || *urgent.Noul != 0.95 {
		t.Errorf("noul answer = %+v", urgent)
	}

	department := resp.Answers["department"]
	if department.Type != decision.TypeChoice || department.Choice != testBillingChoice {
		t.Errorf("choice answer = %+v", department)
	}

	if department.Confidence == nil || *department.Confidence != 0.81 {
		t.Errorf("choice confidence = %v", department.Confidence)
	}

	if department.Probabilities[testBillingChoice] != 0.88 {
		t.Errorf("choice probabilities = %v", department.Probabilities)
	}

	frustration := resp.Answers["frustration"]
	if frustration.Type != decision.TypeScore || frustration.Score == nil || *frustration.Score != 1.05 {
		t.Errorf("score answer = %+v", frustration)
	}

	if frustration.Probabilities["1"] != 0.95 {
		t.Errorf("score probabilities = %v", frustration.Probabilities)
	}

	if frustration.Legend["1"] != "Frustrated" {
		t.Errorf("score legend = %v", frustration.Legend)
	}

	if resp.Usage.InputTokens != 296 || resp.Usage.OutputTokens != 20 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestOpenAI_DecodeResponse_Refusal(t *testing.T) {
	t.Parallel()

	resp, err := decision.OpenAI{}.DecodeResponse([]byte(`{"answers":[{"type":"refusal","name":"risk"}]}`))
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}

	answer, ok := resp.Answers["risk"]
	if !ok {
		t.Fatal("no answer for risk")
	}

	if answer.Type != decision.TypeRefusal {
		t.Errorf("answer type = %q, want refusal", answer.Type)
	}

	if answer.Choice != "" {
		t.Errorf("refusal answer has a choice: %q", answer.Choice)
	}
}

// encode renders a request and returns the wire format as a map.
func encode(t *testing.T, a decision.Adapter, req *decision.Request, model string) map[string]any {
	t.Helper()

	payload, err := a.EncodeRequest(req, model)
	if err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var body map[string]any

	err = json.Unmarshal(data, &body)
	if err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	return body
}

func assertPredicate(t *testing.T, question map[string]any) {
	t.Helper()

	if question["type"] != "predicate" {
		t.Errorf("predicate type = %v, want predicate", question["type"])
	}

	if question["instructions"] != "Does this convey urgency?" {
		t.Errorf("predicate instructions = %v", question["instructions"])
	}

	// The OpenAI predicate type has no criteria field.
	if _, hasCriteria := question["criteria"]; hasCriteria {
		t.Errorf("predicate has criteria: %v", question["criteria"])
	}
}

func assertChoice(t *testing.T, question map[string]any) {
	t.Helper()

	if question["type"] != "choice" {
		t.Errorf("choice type = %v, want choice", question["type"])
	}

	choices, isSlice := question["choices"].([]any)
	if !isSlice || len(choices) != 3 {
		t.Fatalf("choices = %v, want 3 choices", question["choices"])
	}

	first, isObject := choices[0].(map[string]any)
	if !isObject {
		t.Fatalf("choice is not an object: %v", choices[0])
	}

	if first["value"] != testBillingChoice {
		t.Errorf("first choice value = %v, want %s", first["value"], testBillingChoice)
	}

	if first["description"] != "Payments, invoicing, refunds" {
		t.Errorf("first choice description = %v", first["description"])
	}
}

func assertScore(t *testing.T, question map[string]any) {
	t.Helper()

	if question["type"] != "score" {
		t.Errorf("score type = %v, want score", question["type"])
	}

	levels, isSlice := question["levels"].([]any)
	if !isSlice || len(levels) != 3 {
		t.Fatalf("levels = %v, want 3 levels", question["levels"])
	}

	first, isObject := levels[0].(map[string]any)
	if !isObject {
		t.Fatalf("level is not an object: %v", levels[0])
	}

	if first["label"] != "Calm" {
		t.Errorf("first level label = %v, want Calm", first["label"])
	}
}
