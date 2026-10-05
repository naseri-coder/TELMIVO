package ai

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

const VectorDimensions = 96

type Vector []float64

func FeatureVector(text string) Vector {
	v := make(Vector, VectorDimensions)
	tokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, token := range tokens {
		if token == "" {
			continue
		}
		addFeature(v, "w:"+token, 1)
		r := []rune(token)
		for i := 0; i+2 < len(r); i++ {
			addFeature(v, "g:"+string(r[i:i+3]), 0.35)
		}
	}
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range v {
			v[i] /= norm
		}
	}
	return v
}

func addFeature(v Vector, feature string, weight float64) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(feature))
	n := h.Sum64()
	i := int(n % uint64(len(v)))
	if n&(1<<63) != 0 {
		weight = -weight
	}
	v[i] += weight
}

func Cosine(a, b Vector) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
