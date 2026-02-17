package internal

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPreprocessMessagesForTools(t *testing.T) {
	originSignal := FunctionCallTriggerSignal
	FunctionCallTriggerSignal = "<Function_Test_Start/>"
	t.Cleanup(func() {
		FunctionCallTriggerSignal = originSignal
	})

	messages := []Message{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: ToolCallFunction{
					Name:      "calc",
					Arguments: `{"a":1}`,
				},
			}},
		},
		{
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    "42",
		},
	}

	tools := []ToolDefinition{{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "calc",
			Description: "calculator",
			Parameters: map[string]interface{}{
				"type": "object",
			},
		},
	}}

	processed := preprocessMessagesForTools(messages, tools, "required")
	if len(processed) != 3 {
		t.Fatalf("processed length = %d, want 3", len(processed))
	}

	if processed[0].Role != "system" {
		t.Fatalf("processed[0].Role = %q, want system", processed[0].Role)
	}

	systemContent, _ := processed[0].Content.(string)
	if !strings.Contains(systemContent, "You have access to tools.") {
		t.Fatalf("system prompt missing tool instruction: %q", systemContent)
	}
	if !strings.Contains(systemContent, "IMPORTANT: You MUST call at least one tool") {
		t.Fatalf("system prompt missing tool_choice constraint: %q", systemContent)
	}

	assistantContent, _ := processed[1].Content.(string)
	if !strings.Contains(assistantContent, "<function_calls>") {
		t.Fatalf("assistant content missing function calls block: %q", assistantContent)
	}

	toolResult, _ := processed[2].Content.(string)
	if !strings.Contains(toolResult, "<tool_name>calc</tool_name>") {
		t.Fatalf("tool result missing tool name: %q", toolResult)
	}
}

func TestParseFunctionCallsXML(t *testing.T) {
	originSignal := FunctionCallTriggerSignal
	FunctionCallTriggerSignal = "<Function_Test_Start/>"
	t.Cleanup(func() {
		FunctionCallTriggerSignal = originSignal
	})

	content := "先输出一点文本\n<Function_Test_Start/>\n<function_calls>\n<function_call>\n<name>search</name>\n<args_json>{\"q\":\"golang\"}</args_json>\n</function_call>\n</function_calls>"
	calls, pos := ParseFunctionCallsXML(content)
	if pos < 0 {
		t.Fatalf("ParseFunctionCallsXML returned invalid position: %d", pos)
	}
	if len(calls) != 1 {
		t.Fatalf("ParseFunctionCallsXML len = %d, want 1", len(calls))
	}
	if calls[0].Function.Name != "search" {
		t.Fatalf("tool name = %q, want search", calls[0].Function.Name)
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid arguments json: %v", err)
	}
	if got, _ := args["q"].(string); got != "golang" {
		t.Fatalf("arguments.q = %q, want golang", got)
	}
}

func TestDrainSafeAnswerDelta(t *testing.T) {
	answer := "前缀内容<Function_Test_Start/><function_calls></function_calls>"
	delta, emitted, hasTrigger := DrainSafeAnswerDelta(answer, 0, true, "<Function_Test_Start/>")
	if !hasTrigger {
		t.Fatalf("hasTrigger = false, want true")
	}
	if delta != "前缀内容" {
		t.Fatalf("delta = %q, want 前缀内容", delta)
	}
	if emitted != len("前缀内容") {
		t.Fatalf("emitted = %d, want %d", emitted, len("前缀内容"))
	}
}

func TestExtractToolCallsFromPayload(t *testing.T) {
	payload := `{"data":{"phase":"tool_call"},"tool_calls":[{"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":\"beijing\"}"}}]}`
	calls := ExtractToolCallsFromPayload(payload)
	if len(calls) != 1 {
		t.Fatalf("ExtractToolCallsFromPayload len = %d, want 1", len(calls))
	}
	if calls[0].Function.Name != "weather" {
		t.Fatalf("tool name = %q, want weather", calls[0].Function.Name)
	}
}
