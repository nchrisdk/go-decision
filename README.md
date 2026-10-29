# go-decision

Go client for the decision API. The API evaluates a piece of `state`
against a set of questions and returns one answer per question.

Two API dialects:

- `typesafe` — the Typesafe decision API, `POST /v1/systemone`.
- `openai` — the OpenAI decisions API, `POST /v1/decisions`. The client
  translates the canonical request and response to and from OpenAI's format.

Each dialect owns its full path, version included. The base URL stays clean:
`https://api.typesafe.ai`, `https://api.openai.com`. A base URL that already
ends in `/v1` works; the client drops the duplicate.

## Install

```sh
go get github.com/nchrisdk/go-decision
```

## Quickstart

```go
client, err := decision.New(decision.Config{
	APIKey:  os.Getenv("DECISION_API_KEY"),
	APIType: decision.APITypeTypesafe, // or decision.APITypeOpenAI
	BaseURL: "https://api.typesafe.ai",
	Model:   "my-model",
}, http.DefaultClient)
if err != nil {
	log.Fatal(err)
}

req := &decision.Request{
	State: map[string]any{
		"ticket": map[string]any{
			"message":  "Help! My payouts have been failing for 3 days.",
			"age_days": 3,
		},
	},
	Questions: map[string]decision.Question{
		"is_urgent": decision.NewNoulQuestion(
			"Is the payout problem time-sensitive?",
			&decision.NoulCriteria{True: "Time-sensitive", False: "No urgency expressed"},
		),
		"department": decision.NewChoiceQuestion(
			"Which team should handle the payout problem?",
			decision.ChoiceCriteria{
				"billing":   "Payments, invoicing, refunds",
				"technical": "Bugs, outages, integrations",
			},
		),
		"frustration": decision.NewScoreQuestion(
			"How frustrated does the customer sound?",
			decision.ScoreCriteria{"Calm", "Frustrated", "Very angry"},
		),
	},
}

ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()
resp, err := client.Decide(ctx, req)
if err != nil {
	log.Fatal(err)
}

urgent := resp.Answers["is_urgent"]
dept := resp.Answers["department"]
frustr := resp.Answers["frustration"]

// Use probabilities and confidence as decision signals.
if *urgent.Noul > 0.8 && *dept.Confidence > 0.75 {
	fmt.Println("route to", dept.Choice)
}
```

## Question types

| Type     | Criteria                            | Answer                                  |
|----------|-------------------------------------|-----------------------------------------|
| `noul`   | `*NoulCriteria` (`true`/`false`)    | `Noul` — yes-probability, 0–1           |
| `choice` | `ChoiceCriteria` (option → rubric)  | `Choice`, `Probabilities`, `Confidence` |
| `score`  | `ScoreCriteria` (ordered levels)    | `Score`, `Legend`, `Probabilities`, `Confidence` |

`State` and `Instructions` may be a string, object, or array.

## Dialect notes

The canonical format is the typesafe wire format. The `openai` dialect
translates:

- `state` becomes `input` (a string; objects and arrays are JSON-encoded).
- The question map becomes an array, each question with a `name`.
- `noul` becomes `predicate`. OpenAI has no criteria field for it.
- `choice` criteria become `choices` (`value`, `description`).
- `score` criteria become `levels` (`label`, `description`).

## Errors

HTTP failures return `*APIError` with `StatusCode` and `Message`:

```go
if errors.Is(err, decision.ErrRateLimited) { // 429
	// back off
}
if errors.Is(err, decision.ErrOverloaded) { // 529
	// back off
}
var apiErr *decision.APIError
if errors.As(err, &apiErr) {
	// apiErr.StatusCode, apiErr.Message
}
```

## Workflow design

Keep deterministic checks and actions in your application. Put only relevant
context in `State`, ask narrow independent questions in one request, and
combine the answers in code. The client returns model judgments; it does not
choose the workflow or take actions for you.
