package openai

import (
	"slices"
	"testing"
)

func TestModelListIncludesGPTImage2(t *testing.T) {
	if !slices.Contains(ModelList, "gpt-image-2") {
		t.Fatalf("ModelList should include gpt-image-2 for OpenAI-compatible image generation channels")
	}
}
