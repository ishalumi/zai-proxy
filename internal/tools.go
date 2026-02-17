package internal

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	functionCallsBlockPattern = regexp.MustCompile(`(?s)<function_calls>(.*?)</function_calls>`)
	functionCallChunkPattern  = regexp.MustCompile(`(?s)<function_call>(.*?)</function_call>`)
	functionNamePattern       = regexp.MustCompile(`(?s)<name>(.*?)</name>`)
	functionArgsPattern       = regexp.MustCompile(`(?s)<args_json>(.*?)</args_json>`)
	glmToolNamePattern        = regexp.MustCompile(`tool_call_name="([^"]+)"`)
)

// FunctionCallTriggerSignal 用于区分正常回答与工具调用 XML 片段。
var FunctionCallTriggerSignal = "<Function_Go_Start/>"

type toolCallIndexInfo struct {
	name      string
	arguments string
}

func preprocessMessagesForTools(messages []Message, tools []ToolDefinition, toolChoice interface{}) []Message {
	toolIndex := buildToolCallIndex(messages)
	preprocessed := make([]Message, 0, len(messages)+1)

	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "tool":
			contentText := extractTextFromMessageContent(msg.Content)
			info, ok := toolIndex[msg.ToolCallID]
			if !ok {
				info = toolCallIndexInfo{
					name:      firstNonEmpty(msg.Name, "unknown_tool"),
					arguments: "{}",
				}
			}
			preprocessed = append(preprocessed, Message{
				Role:    "user",
				Content: formatToolResultForUpstream(info.name, info.arguments, contentText),
			})
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				xmlCalls := formatAssistantToolCallsForUpstream(msg.ToolCalls, FunctionCallTriggerSignal)
				text, _ := msg.ParseContent()
				content := strings.TrimSpace(strings.TrimSpace(text) + "\n" + xmlCalls)
				preprocessed = append(preprocessed, Message{
					Role:    "assistant",
					Content: content,
				})
				continue
			}
			preprocessed = append(preprocessed, msg)
		case "developer":
			cloned := msg
			cloned.Role = "system"
			preprocessed = append(preprocessed, cloned)
		default:
			preprocessed = append(preprocessed, msg)
		}
	}

	if len(tools) > 0 {
		prompt := generateFunctionPrompt(tools, toolChoice, FunctionCallTriggerSignal)
		if prompt != "" {
			preprocessed = append([]Message{{
				Role:    "system",
				Content: prompt,
			}}, preprocessed...)
		}
	}

	return preprocessed
}

func buildToolCallIndex(messages []Message) map[string]toolCallIndexInfo {
	index := make(map[string]toolCallIndexInfo)
	for _, msg := range messages {
		if strings.ToLower(msg.Role) != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.ID == "" || call.Function.Name == "" {
				continue
			}
			args := normalizeToolArguments(call.Function.Arguments)
			index[call.ID] = toolCallIndexInfo{
				name:      call.Function.Name,
				arguments: args,
			}
		}
	}
	return index
}

func formatToolResultForUpstream(toolName, toolArguments, resultContent string) string {
	return strings.Join([]string{
		"<tool_execution_result>",
		fmt.Sprintf("<tool_name>%s</tool_name>", toolName),
		fmt.Sprintf("<tool_arguments>%s</tool_arguments>", toolArguments),
		fmt.Sprintf("<tool_output>%s</tool_output>", resultContent),
		"</tool_execution_result>",
	}, "\n")
}

func formatAssistantToolCallsForUpstream(toolCalls []ToolCall, triggerSignal string) string {
	if len(toolCalls) == 0 {
		return ""
	}

	var blocks []string
	for _, call := range toolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		args := normalizeToolArguments(call.Function.Arguments)
		blocks = append(blocks, strings.Join([]string{
			"<function_call>",
			fmt.Sprintf("<name>%s</name>", name),
			fmt.Sprintf("<args_json>%s</args_json>", args),
			"</function_call>",
		}, "\n"))
	}
	if len(blocks) == 0 {
		return ""
	}

	return triggerSignal + "\n<function_calls>\n" + strings.Join(blocks, "\n") + "\n</function_calls>"
}

func extractTextFromMessageContent(content interface{}) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			part, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if partType, _ := part["type"].(string); partType == "text" {
				if text, ok := part["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

func generateFunctionPrompt(tools []ToolDefinition, toolChoice interface{}, triggerSignal string) string {
	var toolLines []string
	for i, tool := range tools {
		if tool.Type != "function" || tool.Function.Name == "" {
			continue
		}

		required := "None"
		if req, ok := tool.Function.Parameters["required"].([]interface{}); ok && len(req) > 0 {
			var names []string
			for _, item := range req {
				if name, ok := item.(string); ok && name != "" {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				required = strings.Join(names, ", ")
			}
		}

		paramsJSON := "{}"
		if b, err := json.Marshal(tool.Function.Parameters); err == nil && len(b) > 0 {
			paramsJSON = string(b)
		}

		desc := strings.TrimSpace(tool.Function.Description)
		if desc == "" {
			desc = "None"
		}

		toolLines = append(toolLines, fmt.Sprintf(
			"%d. <tool name=\"%s\">\n   Description: %s\n   Required: %s\n   Parameters JSON Schema: %s",
			i+1,
			tool.Function.Name,
			desc,
			required,
			paramsJSON,
		))
	}

	toolsBlock := "(no tools)"
	if len(toolLines) > 0 {
		toolsBlock = strings.Join(toolLines, "\n\n")
	}

	prompt := strings.Join([]string{
		"You have access to tools.",
		"",
		"When you need to call tools, you MUST output exactly:",
		triggerSignal,
		"<function_calls>",
		"  <function_call>",
		"    <name>tool_name</name>",
		"    <args_json>{\"arg\":\"value\"}</args_json>",
		"  </function_call>",
		"</function_calls>",
		"",
		"Rules:",
		"1) args_json MUST be valid JSON object",
		"2) For multiple calls, output one <function_calls> with multiple <function_call> children",
		"3) If no tool is needed, answer normally",
		"",
		fmt.Sprintf("Available tools:\n%s", toolsBlock),
	}, "\n")

	if extra := processToolChoiceConstraint(toolChoice); extra != "" {
		prompt += extra
	}

	return prompt
}

func processToolChoiceConstraint(toolChoice interface{}) string {
	switch value := toolChoice.(type) {
	case string:
		switch value {
		case "required":
			return "\nIMPORTANT: You MUST call at least one tool in your next response."
		case "none":
			return "\nIMPORTANT: Do not call tools. Answer directly."
		default:
			return ""
		}
	case map[string]interface{}:
		fn, _ := value["function"].(map[string]interface{})
		name, _ := fn["name"].(string)
		if name != "" {
			return "\nIMPORTANT: You MUST call this tool: " + name
		}
	}
	return ""
}

func ParseFunctionCallsXML(text string) ([]ToolCall, int) {
	if text == "" || !strings.Contains(text, FunctionCallTriggerSignal) {
		return nil, -1
	}

	cleaned := removeThinkBlocks(text)
	pos := strings.LastIndex(cleaned, FunctionCallTriggerSignal)
	if pos == -1 {
		return nil, -1
	}

	sub := cleaned[pos:]
	matches := functionCallsBlockPattern.FindStringSubmatch(sub)
	if len(matches) < 2 {
		return nil, -1
	}

	chunks := functionCallChunkPattern.FindAllStringSubmatch(matches[1], -1)
	var calls []ToolCall
	for _, chunk := range chunks {
		if len(chunk) < 2 {
			continue
		}
		nameMatch := functionNamePattern.FindStringSubmatch(chunk[1])
		if len(nameMatch) < 2 {
			continue
		}

		name := strings.TrimSpace(nameMatch[1])
		if name == "" {
			continue
		}

		argsRaw := "{}"
		if argsMatch := functionArgsPattern.FindStringSubmatch(chunk[1]); len(argsMatch) >= 2 {
			argsRaw = strings.TrimSpace(argsMatch[1])
		}

		normalizedArgs := normalizeJSONArguments(argsRaw)
		calls = append(calls, ToolCall{
			ID:   "call_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24],
			Type: "function",
			Function: ToolCallFunction{
				Name:      name,
				Arguments: normalizedArgs,
			},
		})
	}

	if len(calls) == 0 {
		return nil, -1
	}

	return calls, pos
}

func DrainSafeAnswerDelta(answerText string, emittedChars int, hasFunctionCalling bool, triggerSignal string) (string, int, bool) {
	if emittedChars >= len(answerText) {
		return "", emittedChars, false
	}

	if !hasFunctionCalling {
		return answerText[emittedChars:], len(answerText), false
	}

	triggerPos := findLastTriggerSignalOutsideThink(answerText, triggerSignal)
	safeEnd := 0
	hasTrigger := triggerPos >= 0
	if hasTrigger {
		safeEnd = triggerPos
	} else {
		holdBack := len(triggerSignal) - 1
		if holdBack < 0 {
			holdBack = 0
		}
		safeEnd = len(answerText) - holdBack
		if safeEnd < 0 {
			safeEnd = 0
		}
	}

	if safeEnd <= emittedChars {
		return "", emittedChars, hasTrigger
	}

	emittedChars = clampUTF8Boundary(answerText, emittedChars)
	safeEnd = clampUTF8Boundary(answerText, safeEnd)
	if safeEnd <= emittedChars {
		return "", emittedChars, hasTrigger
	}

	return answerText[emittedChars:safeEnd], safeEnd, hasTrigger
}

func clampUTF8Boundary(s string, idx int) int {
	if idx <= 0 {
		return 0
	}
	if idx >= len(s) {
		return len(s)
	}

	for idx > 0 && !utf8.ValidString(s[:idx]) {
		idx--
	}
	return idx
}

func findLastTriggerSignalOutsideThink(text, triggerSignal string) int {
	if text == "" || triggerSignal == "" {
		return -1
	}

	i := 0
	depth := 0
	last := -1
	for i < len(text) {
		switch {
		case strings.HasPrefix(text[i:], "<think>"):
			depth++
			i += len("<think>")
		case strings.HasPrefix(text[i:], "</think>"):
			if depth > 0 {
				depth--
			}
			i += len("</think>")
		case depth == 0 && strings.HasPrefix(text[i:], triggerSignal):
			last = i
			i++
		default:
			i++
		}
	}

	return last
}

func removeThinkBlocks(text string) string {
	for {
		start := strings.Index(text, "<think>")
		if start == -1 {
			break
		}
		pos := start + len("<think>")
		depth := 1
		for pos < len(text) && depth > 0 {
			switch {
			case strings.HasPrefix(text[pos:], "<think>"):
				depth++
				pos += len("<think>")
			case strings.HasPrefix(text[pos:], "</think>"):
				depth--
				pos += len("</think>")
			default:
				pos++
			}
		}
		if depth != 0 {
			break
		}
		text = text[:start] + text[pos:]
	}
	return text
}

func ExtractToolCallsFromPayload(payload string) []ToolCall {
	if payload == "" {
		return nil
	}

	var calls []ToolCall
	calls = MergeToolCalls(calls, parseJSONToolCalls(payload))
	calls = MergeToolCalls(calls, parseJSONFunctionCall(payload))
	calls = MergeToolCalls(calls, parseGlmBlockToolCalls(payload))
	return calls
}

func MergeToolCalls(existing []ToolCall, incoming []ToolCall) []ToolCall {
	if len(incoming) == 0 {
		return existing
	}

	seen := make(map[string]struct{}, len(existing))
	for _, call := range existing {
		seen[toolCallUniqueKey(call)] = struct{}{}
	}

	for _, call := range incoming {
		normalized := normalizeToolCall(call)
		key := toolCallUniqueKey(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, normalized)
	}

	return existing
}

func parseJSONToolCalls(payload string) []ToolCall {
	arrayJSON := extractJSONArrayByKey(payload, `"tool_calls":`)
	if arrayJSON == "" {
		return nil
	}

	var rawCalls []map[string]interface{}
	if err := json.Unmarshal([]byte(arrayJSON), &rawCalls); err != nil {
		return nil
	}

	var calls []ToolCall
	for _, raw := range rawCalls {
		if call, ok := parseSingleToolCallMap(raw); ok {
			calls = append(calls, call)
		}
	}
	return calls
}

func parseJSONFunctionCall(payload string) []ToolCall {
	objJSON := extractJSONObjectByKey(payload, `"function_call":`)
	if objJSON == "" {
		return nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(objJSON), &raw); err != nil {
		return nil
	}

	name, _ := raw["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil
	}

	args := "{}"
	if v, ok := raw["arguments"]; ok {
		switch t := v.(type) {
		case string:
			args = normalizeToolArguments(t)
		default:
			if b, err := json.Marshal(t); err == nil {
				args = normalizeToolArguments(string(b))
			}
		}
	}

	return []ToolCall{{
		Type: "function",
		Function: ToolCallFunction{
			Name:      strings.TrimSpace(name),
			Arguments: args,
		},
	}}
}

func parseGlmBlockToolCalls(payload string) []ToolCall {
	matches := glmToolNamePattern.FindAllStringSubmatch(payload, -1)
	if len(matches) == 0 {
		return nil
	}

	var calls []ToolCall
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		calls = append(calls, ToolCall{
			Type: "function",
			Function: ToolCallFunction{
				Name:      name,
				Arguments: "{}",
			},
		})
	}
	return calls
}

func parseSingleToolCallMap(raw map[string]interface{}) (ToolCall, bool) {
	call := ToolCall{
		Type: "function",
	}

	if id, ok := raw["id"].(string); ok {
		call.ID = id
	}
	if t, ok := raw["type"].(string); ok && t != "" {
		call.Type = t
	}

	fnRaw, fnOK := raw["function"].(map[string]interface{})
	if fnOK {
		if name, ok := fnRaw["name"].(string); ok {
			call.Function.Name = strings.TrimSpace(name)
		}
		if args, ok := fnRaw["arguments"]; ok {
			switch value := args.(type) {
			case string:
				call.Function.Arguments = normalizeToolArguments(value)
			default:
				if b, err := json.Marshal(value); err == nil {
					call.Function.Arguments = normalizeToolArguments(string(b))
				}
			}
		}
	}

	if call.Function.Name == "" {
		if name, ok := raw["name"].(string); ok {
			call.Function.Name = strings.TrimSpace(name)
		}
	}

	if call.Function.Name == "" {
		return ToolCall{}, false
	}
	if call.Function.Arguments == "" {
		call.Function.Arguments = "{}"
	}

	return normalizeToolCall(call), true
}

func extractJSONArrayByKey(content, key string) string {
	idx := strings.Index(content, key)
	if idx == -1 {
		return ""
	}

	start := idx + len(key)
	for start < len(content) && content[start] != '[' {
		start++
	}
	if start >= len(content) {
		return ""
	}

	end := findJSONCompositeEnd(content, start, '[', ']')
	if end == -1 {
		return ""
	}
	return content[start:end]
}

func extractJSONObjectByKey(content, key string) string {
	idx := strings.Index(content, key)
	if idx == -1 {
		return ""
	}

	start := idx + len(key)
	for start < len(content) && content[start] != '{' {
		start++
	}
	if start >= len(content) {
		return ""
	}

	end := findJSONCompositeEnd(content, start, '{', '}')
	if end == -1 {
		return ""
	}
	return content[start:end]
}

func findJSONCompositeEnd(content string, start int, left byte, right byte) int {
	depth := 0
	inString := false
	escapeNext := false

	for i := start; i < len(content); i++ {
		ch := content[i]
		if escapeNext {
			escapeNext = false
			continue
		}
		if ch == '\\' {
			escapeNext = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == left {
			depth++
		}
		if ch == right {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}

	return -1
}

func normalizeToolCall(call ToolCall) ToolCall {
	call.Type = firstNonEmpty(call.Type, "function")
	if call.ID == "" {
		call.ID = "call_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
	}
	call.Function.Name = strings.TrimSpace(call.Function.Name)
	call.Function.Arguments = normalizeToolArguments(call.Function.Arguments)
	return call
}

func toolCallUniqueKey(call ToolCall) string {
	id := strings.TrimSpace(call.ID)
	if id != "" {
		return "id:" + id
	}
	return "payload:" + call.Function.Name + "|" + normalizeToolArguments(call.Function.Arguments)
}

func normalizeToolArguments(arguments string) string {
	if strings.TrimSpace(arguments) == "" {
		return "{}"
	}
	return normalizeJSONArguments(arguments)
}

func normalizeJSONArguments(raw string) string {
	var decoded interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		fallback := map[string]interface{}{"raw": raw}
		b, _ := json.Marshal(fallback)
		return string(b)
	}

	switch value := decoded.(type) {
	case map[string]interface{}:
		b, _ := json.Marshal(value)
		return string(b)
	default:
		fallback := map[string]interface{}{"value": value}
		b, _ := json.Marshal(fallback)
		return string(b)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
