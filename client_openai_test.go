package decision_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/nchrisdk/go-decision"
)

const openAIResponseBody = `{
	"model": "gpt-6-luna",
	"answers": [
		{"type": "predicate", "name": "is_urgent", "probability": 0.95},
		{"type": "choice", "name": "department", "choice": "billing",
			"probabilities": [{"value": "billing", "probability": 0.88},
				{"value": "technical", "probability": 0.12}],
			"confidence": 0.81},
		{"type": "score", "name": "frustration", "score": 1.05,
			"probabilities": [{"value": 0, "label": "Calm", "probability": 0.0},
				{"value": 1, "label": "Frustrated", "probability": 0.95},
				{"value": 2, "label": "Very angry", "probability": 0.05}],
			"confidence": 0.92}
	],
	"usage": {"input_tokens": 296, "output_tokens": 20}
}`

func TestOpenAIDecide(t *testing.T) {
	t.Parallel()

	var (
		gotPath, gotMethod string
		gotBody            []byte
	)

	srv := testClient(t, func(res http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotMethod = req.Method
		gotBody, _ = io.ReadAll(req.Body)

		res.Header().Set("Content-Type", "application/json")
		_, _ = res.Write([]byte(openAIResponseBody))
	})

	client := newClient(t, srv, decision.APITypeOpenAI, "gpt-6-luna")

	resp, err := client.Decide(context.Background(), allTypesRequest())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v1/decisions" {
		t.Errorf("got %s %s, want POST /v1/decisions", gotMethod, gotPath)
	}

	var body map[string]any

	err = json.Unmarshal(gotBody, &body)
	if err != nil {
		t.Fatalf("decode request body: %v", err)
	}

	if body["model"] != "gpt-6-luna" {
		t.Errorf("model = %v, want the configured model %q", body["model"], "gpt-6-luna")
	}

	if body["input"] != testState {
		t.Errorf("input = %v, want %q", body["input"], testState)
	}

	if resp.Answers["department"].Choice != testBillingChoice {
		t.Errorf("choice answer = %+v", resp.Answers["department"])
	}
}

func TestOpenAIDecide_NoModel(t *testing.T) {
	t.Parallel()

	srv := testClient(t, func(res http.ResponseWriter, _ *http.Request) {
		_, _ = res.Write([]byte(openAIResponseBody))
	})

	// The OpenAI API requires a model; without one the client refuses to
	// send a request that the API would reject.
	client := newClient(t, srv, decision.APITypeOpenAI, "")

	_, err := client.Decide(context.Background(), &decision.Request{
		State:     "hi",
		Questions: map[string]decision.Question{},
	})
	if err == nil {
		t.Fatal("want an error when no model is set")
	}

	if !strings.Contains(err.Error(), "requires a model") {
		t.Errorf("error = %v, want a missing model error", err)
	}
}

func TestOpenAIDecide_BaseURLWithVersion(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := testClient(t, func(res http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path

		_, _ = res.Write([]byte(openAIResponseBody))
	})

	// A base URL that already carries the version prefix must not double it.
	client, err := decision.New(decision.Config{
		APIKey:  "test-key",
		APIType: decision.APITypeOpenAI,
		BaseURL: srv.URL + "/v1",
		Model:   "gpt-6-luna",
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Decide(context.Background(), &decision.Request{
		State:     "hi",
		Questions: map[string]decision.Question{},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if gotPath != "/v1/decisions" {
		t.Errorf("path = %q, want /v1/decisions", gotPath)
	}
}

func TestOpenAIDecide_MissingAnswer(t *testing.T) {
	t.Parallel()

	srv := testClient(t, func(res http.ResponseWriter, _ *http.Request) {
		_, _ = res.Write([]byte(`{"model":"gpt-6-luna","answers":[]}`))
	})

	client := newClient(t, srv, decision.APITypeOpenAI, "gpt-6-luna")

	_, err := client.Decide(context.Background(), &decision.Request{
		State: "hi",
		Questions: map[string]decision.Question{
			"risk": decision.NewChoiceQuestion("Is it safe?", decision.ChoiceCriteria{"safe": "Fine"}),
		},
	})
	if err == nil {
		t.Fatal("want an error for a missing answer")
	}
}
