package llm

import "testing"

func TestParseModelListOpenAIAndOllama(t *testing.T) {
	got, err := parseModelList([]byte(`{"data":[{"id":"zeta"},{"id":"alpha"},{"id":"alpha"}]}`))
	if err != nil || len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("%v %v", got, err)
	}
	got, err = parseModelList([]byte(`{"models":[{"name":"llama3.2:latest"},{"name":"mistral"}]}`))
	if err != nil || len(got) != 2 || got[0] != "llama3.2:latest" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := parseModelList([]byte(`{"data":[]}`)); err == nil {
		t.Fatal("expected empty list to fail")
	}
}
