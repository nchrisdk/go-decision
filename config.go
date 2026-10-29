package decision

// Config holds the settings for a client.
type Config struct {
	// APIKey is the API key, sent as a Bearer token.
	APIKey string
	// APIType selects the API dialect: typesafe or openai.
	APIType APIType
	// BaseURL is the base URL of the API, without the version. The adapter
	// appends the full API path, including the version.
	BaseURL string
	// Model is the model the API uses.
	Model string
}
