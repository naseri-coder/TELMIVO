package ai

import "testing"

func TestFeatureVectorDeterministicAndBounded(t *testing.T) {
	a := FeatureVector("refund policy thirty days")
	b := FeatureVector("refund policy thirty days")
	if len(a) != VectorDimensions || len(b) != VectorDimensions {
		t.Fatal("unexpected vector dimensions")
	}
	if Cosine(a, b) < 0.999999 {
		t.Fatal("identical text did not preserve similarity")
	}
	if Cosine(a, FeatureVector("refund receipt support")) <= Cosine(a, FeatureVector("volcano telescope granite")) {
		t.Fatal("related text did not rank above unrelated text")
	}
	if Cosine(nil, a) != 0 {
		t.Fatal("invalid vector comparison must be zero")
	}
}
