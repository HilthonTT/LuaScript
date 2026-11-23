package ml

import "testing"

func TestUnmarshalRejectsMalformedModels(t *testing.T) {
	cases := map[string]string{
		"no config":        `{}`,
		"empty layout":     `{"Config":{"Inputs":2,"Layout":[]}}`,
		"negative inputs":  `{"Config":{"Inputs":-2,"Layout":[1]},"Weights":[[[1,2]]]}`,
		"zero layer":       `{"Config":{"Inputs":2,"Layout":[0]},"Weights":[[]]}`,
		"negative layer":   `{"Config":{"Inputs":2,"Layout":[-1]},"Weights":[]}`,
		"huge layers":      `{"Config":{"Inputs":1,"Layout":[100000,100000]},"Weights":[]}`,
		"overflow layers":  `{"Config":{"Inputs":4611686018427387904,"Layout":[4]},"Weights":[]}`,
		"bad activation":   `{"Config":{"Inputs":2,"Layout":[1],"Activation":99},"Weights":[[[1,2]]]}`,
		"weights missing":  `{"Config":{"Inputs":2,"Layout":[1]},"Weights":[]}`,
		"weights short":    `{"Config":{"Inputs":2,"Layout":[1]},"Weights":[[[1]]]}`,
		"weights no layer": `{"Config":{"Inputs":2,"Layout":[2]},"Weights":[[[1,2]]]}`,
	}
	for name, blob := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Unmarshal panicked: %v", r)
				}
			}()
			if _, err := Unmarshal([]byte(blob)); err == nil {
				t.Fatalf("Unmarshal(%s) succeeded; want an error", blob)
			}
		})
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	n := NewNeural(&Config{Inputs: 3, Layout: []int{4, 2}, Bias: true})
	blob, err := n.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Unmarshal(blob)
	if err != nil {
		t.Fatalf("Unmarshal of a marshalled network: %v", err)
	}
	if back.NumWeights() != n.NumWeights() {
		t.Fatalf("round trip changed weight count: %d -> %d", n.NumWeights(), back.NumWeights())
	}
}
