package executor

import (
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestAntigravityImageGenerationRequest(t *testing.T) {
	if !antigravityImageGenerationRequest(cliproxyexecutor.Options{Alt: "images/generations"}) {
		t.Fatal("expected legacy alt image generation request to match")
	}

	if !antigravityImageGenerationRequest(cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-image"),
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/images/generations",
		},
	}) {
		t.Fatal("expected openai-image generation request path to match")
	}

	if antigravityImageGenerationRequest(cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-image"),
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/images/edits",
		},
	}) {
		t.Fatal("did not expect image edits path to match generations")
	}
}

func TestOpenAIImageGenerationRequestToChatCompletions(t *testing.T) {
	out := openAIImageGenerationRequestToChatCompletions("gemini-3.1-flash-image", []byte(`{"model":"gpt-image-2","prompt":"small kitten","size":"1024x1024"}`))

	if got := gjson.GetBytes(out, "model").String(); got != "gemini-3.1-flash-image" {
		t.Fatalf("model = %q", got)
	}
	if got := gjson.GetBytes(out, "messages.0.content").String(); got != "small kitten" {
		t.Fatalf("prompt = %q", got)
	}
	if got := gjson.GetBytes(out, "modalities.0").String(); got != "image" {
		t.Fatalf("modalities.0 = %q", got)
	}
	if got := gjson.GetBytes(out, "image_config.aspect_ratio").String(); got != "1:1" {
		t.Fatalf("aspect ratio = %q", got)
	}
}

func TestOpenAIChatCompletionsResponseToImageGeneration(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,a2l0dGVu"}}]}}]}`)
	out := openAIChatCompletionsResponseToImageGeneration(raw)

	if got := gjson.GetBytes(out, "data.0.b64_json").String(); got != "a2l0dGVu" {
		t.Fatalf("b64_json = %q", got)
	}
}
