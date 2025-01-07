package everything

type EchoArgs struct {
	Message string `json:"message"`
}

type AddArgs struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

type LongRunningOperationArgs struct {
	Duration float64 `json:"duration"`
	Steps    float64 `json:"steps"`
}

type SampleLLMArgs struct {
	Prompt    string  `json:"prompt"`
	MaxTokens float64 `json:"maxTokens"`
}

var echoSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "message": { "type": "string" }
    }
  }
`)

var addSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "a": { "type": "number" },
      "b": { "type": "number" }
    }
  }
`)

var longRunningOperationSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "duration": { "type": "number", "default": 10 },
      "steps": { "type": "number", "default": 5 }
    }
  }
`)

var sampleLLMSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "prompt": { "type": "string" },
      "maxTokens": { "type": "number", "default": 100 }
    }
  }
`)
