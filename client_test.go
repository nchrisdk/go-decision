package decision_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nchrisdk/go-decision"
)

const (
	testModel         = "latest"
	testBillingChoice = "billing"

	questionTypeNoul   = "noul"
	questionTypeChoice = "choice"
	questionTypeScore  = "score"

	testState = "Help! My payouts have been failing for 3 days."

	// exampleResponse has one answer of each question type.
	exampleResponse = `{
		"model": "test-model",
		"answers": {
			"is_urgent": {"type": "noul", "noul": 0.95},
			"department": {"type": "choice", "choice": "billing",
				"probabilities": {"billing": 0.88, "technical": 0.12, "sales": 0.0},
				"confidence": 0.81},
			"frustration": {"type": "score", "score": 1.05,
				"legend": {"0": "Calm", "1": "Frustrated", "2": "Very angry"},
				"probabilities": {"0": 0.0, "1": 0.95, "2": 0.05},
				"confidence": 0.92}
		},
		"usage": {"input_tokens": 296, "output_tokens": 20}
	}`
)

func testClient(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv
}

func newClient(t *testing.T, srv *httptest.Server, apiType decision.APIType, model string) *decision.Client {
	t.Helper()

	client, err := decision.New(decision.Config{APIKey: "test-key", APIType: apiType, BaseURL: srv.URL, Model: model}, http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return client
}

func allTypesRequest() *decision.Request {
	return &decision.Request{
		State: testState,
		Questions: map[string]decision.Question{
			"is_urgent": decision.NewNoulQuestion(
				"Does this convey urgency?",
				&decision.NoulCriteria{
					True:  "Explicitly time-sensitive",
					False: "No urgency expressed",
				},
			),
			"department": decision.NewChoiceQuestion(
				"Which team should handle this?",
				decision.ChoiceCriteria{
					testBillingChoice: "Payments, invoicing, refunds",
					"technical":       "Bugs, outages, integrations",
					"sales":           "Pricing, upgrades, new accounts",
				},
			),
			"frustration": decision.NewScoreQuestion(
				"How frustrated is the customer?",
				decision.ScoreCriteria{"Calm", "Frustrated", "Very angry"},
			),
		},
	}
}

func TestNew_UnknownAPIType(t *testing.T) {
	for _, apiType := range []decision.APIType{"", "nope"} {
		if _, err := decision.New(decision.Config{APIType: apiType}, http.DefaultClient); err == nil {
			t.Errorf("APIType %q: want an error, got nil", apiType)
		}
	}
}

func TestNew_EmptyBaseURL(t *testing.T) {
	if _, err := decision.New(decision.Config{APIType: decision.APITypeTypesafe}, http.DefaultClient); err == nil {
		t.Error("want an error when no base URL is set")
	}
}

func TestNew_NilClient(t *testing.T) {
	if _, err := decision.New(decision.Config{APIType: decision.APITypeTypesafe, BaseURL: "http://localhost"}, nil); err == nil {
		t.Error("want an error when no http client is set")
	}
}

func TestDecide_AllQuestionTypes(t *testing.T) {
	t.Parallel()

	var (
		gotAuth, gotContentType, gotMethod, gotPath string
		gotBody                                     []byte
	)

	srv := testClient(t, func(res http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		gotBody = data
		gotAuth = req.Header.Get("Authorization")
		gotContentType = req.Header.Get("Content-Type")
		gotMethod = req.Method
		gotPath = req.URL.Path

		res.Header().Set("Content-Type", "application/json")

		_, _ = fmt.Fprint(res, exampleResponse)
	})
	client := newClient(t, srv, decision.APITypeTypesafe, testModel)

	resp, err := client.Decide(context.Background(), allTypesRequest())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v1/systemone" {
		t.Errorf("got %s %s, want POST /v1/systemone", gotMethod, gotPath)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}

	assertRequestJSON(t, gotBody)
	assertAnswers(t, resp)
}

func TestDecide_Model(t *testing.T) {
	t.Parallel()

	var gotBody []byte

	srv := testClient(t, func(res http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		gotBody = data

		_, _ = fmt.Fprint(res, `{"model":"test-model","answers":{},"usage":{"input_tokens":1,"output_tokens":1}}`)
	})
	client := newClient(t, srv, decision.APITypeTypesafe, testModel)

	_, err := client.Decide(context.Background(), &decision.Request{State: "hi", Questions: map[string]decision.Question{}})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	var body struct {
		Model string `json:"model"`
	}

	err = json.Unmarshal(gotBody, &body)
	if err != nil {
		t.Fatalf("decode request body: %v", err)
	}

	if body.Model != testModel {
		t.Errorf("model sent = %q, want the configured model %q", body.Model, testModel)
	}
}

func TestDecide_MissingAnswer(t *testing.T) {
	t.Parallel()

	srv := testClient(t, func(res http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(res, `{"model":"test-model","answers":{},"usage":{"input_tokens":1,"output_tokens":1}}`)
	})
	client := newClient(t, srv, decision.APITypeTypesafe, testModel)
	request := &decision.Request{
		Questions: map[string]decision.Question{"question-id": {Type: questionTypeNoul}},
	}

	_, err := client.Decide(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), `no answer for "question-id"`) {
		t.Fatalf("missing answer error = %v", err)
	}
}

func TestDecide_ErrorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status  int
		body    string
		message string
		wantErr error
	}{
		{status: http.StatusUnauthorized, body: `{"error":"missing or invalid API key"}`, message: `{"error":"missing or invalid API key"}`},
		{status: http.StatusUnprocessableEntity, body: `{"error":"question 'x' has no criteria"}`, message: `{"error":"question 'x' has no criteria"}`},
		{status: http.StatusBadGateway, body: `upstream unavailable`, message: `upstream unavailable`},
		{status: http.StatusTooManyRequests, body: `{"error":"rate limited"}`, message: `{"error":"rate limited"}`, wantErr: decision.ErrRateLimited},
		{status: 529, body: `{"error":"overloaded"}`, message: `{"error":"overloaded"}`, wantErr: decision.ErrOverloaded},
	}

	for _, testCase := range tests {
		srv := testClient(t, func(res http.ResponseWriter, _ *http.Request) {
			res.WriteHeader(testCase.status)

			_, _ = res.Write([]byte(testCase.body))
		})
		client := newClient(t, srv, decision.APITypeTypesafe, testModel)

		_, err := client.Decide(context.Background(), &decision.Request{State: "hi", Questions: map[string]decision.Question{}})
		if err == nil {
			t.Fatalf("status %d: expected error, got nil", testCase.status)
		}

		var apiErr *decision.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("status %d: error has type %T, want *decision.APIError", testCase.status, err)
		}

		if apiErr.StatusCode != testCase.status || apiErr.Message != testCase.message {
			t.Errorf("APIError = %+v, want status %d and message %q", apiErr, testCase.status, testCase.message)
		}

		if !strings.Contains(err.Error(), strconv.Itoa(testCase.status)) || !strings.Contains(err.Error(), testCase.message) {
			t.Errorf("error = %q, want status and message", err)
		}

		if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
			t.Errorf("error = %v, want errors.Is(_, %v)", err, testCase.wantErr)
		}
	}
}

func TestDecide_LargeResponse(t *testing.T) {
	t.Parallel()

	padding := strings.Repeat("x", 2<<20)
	body := fmt.Sprintf(`{"model":"test-model","answers":{"x":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1,"output_tokens":1},"padding":%q}`, padding)
	srv := testClient(t, func(res http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(res, body)
	})
	client := newClient(t, srv, decision.APITypeTypesafe, testModel)

	resp, err := client.Decide(context.Background(), &decision.Request{
		State:     "hi",
		Questions: map[string]decision.Question{"x": {Type: questionTypeNoul}},
	})
	if err != nil {
		t.Fatalf("Decide large response: %v", err)
	}

	if resp.Answers["x"].Noul == nil || *resp.Answers["x"].Noul != 0.5 {
		t.Fatalf("answer = %+v", resp.Answers["x"])
	}
}

func TestDecide_ResponseExceedsLimit(t *testing.T) {
	t.Parallel()

	padding := strings.Repeat("x", 33<<20)
	body := fmt.Sprintf(`{"model":"test-model","answers":{},"usage":{"input_tokens":1,"output_tokens":1},"padding":%q}`, padding)
	srv := testClient(t, func(res http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(res, body)
	})
	client := newClient(t, srv, decision.APITypeTypesafe, testModel)

	_, err := client.Decide(context.Background(), &decision.Request{State: "hi", Questions: map[string]decision.Question{}})
	if err == nil {
		t.Fatal("want an error for a response over 32 MiB")
	}

	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want the size limit error", err)
	}
}

func TestDecide_CustomHTTPClient(t *testing.T) {
	t.Parallel()

	var calls int

	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"test-model","answers":{},"usage":{"input_tokens":1,"output_tokens":1}}`)),
		}, nil
	})}

	client, err := decision.New(decision.Config{
		APIKey:  "test-key",
		APIType: decision.APITypeTypesafe,
		BaseURL: "http://unused",
		Model:   testModel,
	}, httpClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := client.Decide(context.Background(), &decision.Request{State: "hi", Questions: map[string]decision.Question{}})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if calls != 1 {
		t.Errorf("custom transport called %d times, want 1", calls)
	}

	if resp.Model != "test-model" {
		t.Errorf("model = %q, want %q", resp.Model, "test-model")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func assertRequestJSON(t *testing.T, body []byte) {
	t.Helper()

	var got map[string]any

	err := json.Unmarshal(body, &got)
	if err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}

	if got["state"] != testState {
		t.Errorf("state = %v", got["state"])
	}

	if got["model"] != testModel {
		t.Errorf("model = %v", got["model"])
	}

	questions, isMap := got["questions"].(map[string]any)
	if !isMap {
		t.Fatalf("questions is not an object")
	}

	assertNoulRequest(t, questions["is_urgent"])
	assertChoiceRequest(t, questions["department"])
	assertScoreRequest(t, questions["frustration"])
}

func assertNoulRequest(t *testing.T, q any) {
	t.Helper()

	obj, isObj := q.(map[string]any)
	if !isObj {
		t.Fatalf("noul question is not an object: %v", q)
	}

	if obj["type"] != questionTypeNoul {
		t.Errorf("noul type = %v", obj["type"])
	}

	crit, isCrit := obj["criteria"].(map[string]any)
	if !isCrit {
		t.Fatalf("noul criteria is not an object: %v", obj["criteria"])
	}

	if crit["true"] != "Explicitly time-sensitive" || crit["false"] != "No urgency expressed" {
		t.Errorf("noul criteria = %v", crit)
	}
}

func assertChoiceRequest(t *testing.T, q any) {
	t.Helper()

	obj, isObj := q.(map[string]any)
	if !isObj {
		t.Fatalf("choice question is not an object: %v", q)
	}

	if obj["type"] != questionTypeChoice {
		t.Errorf("choice type = %v", obj["type"])
	}

	crit, isCrit := obj["criteria"].(map[string]any)
	if !isCrit {
		t.Fatalf("choice criteria is not an object: %v", obj["criteria"])
	}

	if len(crit) != 3 || crit[testBillingChoice] != "Payments, invoicing, refunds" {
		t.Errorf("choice criteria = %v", crit)
	}
}

func assertScoreRequest(t *testing.T, q any) {
	t.Helper()

	obj, isObj := q.(map[string]any)
	if !isObj {
		t.Fatalf("score question is not an object: %v", q)
	}

	if obj["type"] != questionTypeScore {
		t.Errorf("score type = %v", obj["type"])
	}

	crit, isArr := obj["criteria"].([]any)
	if !isArr {
		t.Fatalf("score criteria is not an array: %v", obj["criteria"])
	}

	if len(crit) != 3 || crit[0] != "Calm" {
		t.Errorf("score criteria = %v", crit)
	}
}

func assertAnswers(t *testing.T, resp *decision.Response) {
	t.Helper()

	if resp.Model != "test-model" {
		t.Errorf("model = %q, want %q", resp.Model, "test-model")
	}

	assertNoulAnswer(t, resp.Answers["is_urgent"])
	assertChoiceAnswer(t, resp.Answers["department"])
	assertScoreAnswer(t, resp.Answers["frustration"])

	if resp.Usage.InputTokens != 296 || resp.Usage.OutputTokens != 20 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func assertNoulAnswer(t *testing.T, answer decision.Answer) {
	t.Helper()

	if answer.Type != questionTypeNoul || answer.Noul == nil || *answer.Noul != 0.95 {
		t.Errorf("noul answer = %+v", answer)
	}

	if answer.Choice != "" || answer.Score != nil || answer.Legend != nil {
		t.Errorf("noul answer has unexpected fields set: %+v", answer)
	}
}

func assertChoiceAnswer(t *testing.T, answer decision.Answer) {
	t.Helper()

	if answer.Type != questionTypeChoice || answer.Choice != testBillingChoice || answer.Confidence == nil || *answer.Confidence != 0.81 {
		t.Errorf("choice answer = %+v", answer)
	}

	if answer.Probabilities[testBillingChoice] != 0.88 || answer.Probabilities["sales"] != 0.0 {
		t.Errorf("choice probabilities = %v", answer.Probabilities)
	}
}

func assertScoreAnswer(t *testing.T, answer decision.Answer) {
	t.Helper()

	if answer.Type != questionTypeScore || answer.Score == nil || *answer.Score != 1.05 {
		t.Errorf("score answer = %+v", answer)
	}

	if answer.Legend["1"] != "Frustrated" || answer.Probabilities["1"] != 0.95 {
		t.Errorf("score legend/probabilities = %+v", answer)
	}

	if answer.Confidence == nil || *answer.Confidence != 0.92 {
		t.Errorf("score confidence = %+v", answer.Confidence)
	}
}
