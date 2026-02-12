package internal

import "testing"

func TestParseModelNameTags(t *testing.T) {
	tests := []struct {
		model          string
		wantBaseModel  string
		wantThinking   bool
		wantSearchMode bool
	}{
		{model: "GLM-5", wantBaseModel: "GLM-5"},
		{model: "GLM-5-thinking", wantBaseModel: "GLM-5", wantThinking: true},
		{model: "GLM-5-search", wantBaseModel: "GLM-5", wantSearchMode: true},
		{model: "GLM-5-thinking-search", wantBaseModel: "GLM-5", wantThinking: true, wantSearchMode: true},
		{model: "GLM-5-search-thinking", wantBaseModel: "GLM-5", wantThinking: true, wantSearchMode: true},
	}

	for _, tt := range tests {
		baseModel, thinking, searchMode := ParseModelName(tt.model)
		if baseModel != tt.wantBaseModel || thinking != tt.wantThinking || searchMode != tt.wantSearchMode {
			t.Fatalf("ParseModelName(%q) = (%q, %v, %v), want (%q, %v, %v)",
				tt.model, baseModel, thinking, searchMode, tt.wantBaseModel, tt.wantThinking, tt.wantSearchMode)
		}
	}
}

func TestGetTargetModelFallbackUsesBaseModel(t *testing.T) {
	if got := GetTargetModel("glm-5-search"); got != "glm-5" {
		t.Fatalf("GetTargetModel(glm-5-search) = %q, want %q", got, "glm-5")
	}
}

func TestIsToolCallPayload(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{content: `<glm_block tool_call_name="retrieve">...</glm_block>`, want: true},
		{content: `{"type":"mcp","data":{"mcp_server":{"name":"mcp-server"}}}`, want: true},
		{content: `{"tool_calls":[{"id":"call_1"}]}`, want: true},
		{content: `{"function_call":{"name":"foo"}}`, want: true},
		{content: `普通回答内容`, want: false},
	}

	for _, tt := range tests {
		if got := IsToolCallPayload(tt.content); got != tt.want {
			t.Fatalf("IsToolCallPayload(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}
